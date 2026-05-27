# CLAUDE.md - uncloud-deployer

This document helps AI assistants understand the codebase and contribute effectively.

## Project overview

`uncloud-deployer` is a continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside an Uncloud cluster and deploys services in response to GitHub push webhooks.

The deployer connects to the Uncloud daemon through its Unix socket (`/run/uncloud/uncloud.sock`), which every node in the cluster exposes. This gives the deployer full cluster access without needing SSH keys or network configuration.

## Repository structure

```
main.go         Entry point. Loads config, registers webhook routes, runs the HTTP server.
config.go       Loads AppConfig from a YAML file (DEPLOYER_CONFIG) or environment variables.
webhook.go      GitHub webhook handler. Reads body, verifies HMAC, dispatches to deployer.
deployer.go     Core deploy logic. Connects to socket, loads compose file, plans and executes deploy.
build.go        Builds services with a build directive and pushes images to cluster machines.
repo.go         Clones or fetches the repository at the push commit (repo mode only).
Dockerfile      Multi-stage build. Build context is the deployer directory.
compose.yaml    Reference compose file for deploying the deployer itself into Uncloud.
mise.toml       Build tooling. Go 1.26.1 via mise. Tasks: build, run, build:image.
misc/design.md  Architecture decisions and options considered during design.
```

## Key types

`AppConfig` — top-level config with `ListenAddr` and `Targets []TargetConfig`.

`TargetConfig` — per-target settings: `Name`, `WebhookSecret`, `Branch`, `ComposeFile`, `ForceRecreate`, `RepoURL`, `RepoToken`, `WorkDir`, `SocketPath`. Each `Deployer` holds one `TargetConfig`.

## Configuration

Two modes:

**YAML file** (`DEPLOYER_CONFIG` points to a file): supports multiple targets, each registered at `/webhook/<name>`.

**Environment variables** (no `DEPLOYER_CONFIG`): single target, registered at `/webhook` for backward compatibility. `loadEnvConfig` builds a `TargetConfig` from `DEPLOYER_*` env vars.

`loadFileConfig` applies defaults: `branch` → `main`, `work_dir` → `/deploy/work/<name>`, `compose_file` → `compose.yaml` if `repo_url` is set, `/deploy/compose.yaml` otherwise. It also validates that every target has a unique non-empty name and copies `socket_path` from the global config into each `TargetConfig`.

## Key dependencies

The deployer imports `github.com/psviderski/uncloud` as a standard Go module dependency pinned to a specific commit. The uncloud packages used directly:

| Package | Purpose |
|---|---|
| `pkg/client` | `client.New` creates a cluster client from a connector |
| `pkg/client/connector` | `NewUnixConnector` connects to the daemon socket |
| `pkg/client/compose` | `LoadProject` and `NewDeploymentWithStrategy` implement `uc deploy` logic |
| `pkg/client/deploy` | `RollingStrategy` controls how containers are updated |

In repo mode, the deployer also uses these directly (both are transitive dependencies of uncloud):

| Package | Purpose |
|---|---|
| `github.com/docker/cli/cli/command` | Creates a Docker CLI client for the build step |
| `github.com/docker/compose/v2/pkg/compose` | Builds images via the Compose Go library |

`internal/cli.BuildServices` in uncloud contains equivalent build logic but is not importable from outside the module. `build.go` replicates the relevant parts. See the TODO comment there for the long-term option.

## Deploy flow

1. `webhook.go` receives a POST to `/webhook` (env-var mode) or `/webhook/<name>` (YAML mode).
2. HMAC signature is verified against the target's `WebhookSecret`.
3. The event is checked: must be a push to the configured branch.
4. `triggerDeploy` sends the event to the deployer's channel (capacity 1). A second concurrent event is queued. A third is dropped with a warning.
5. `deployLoop` runs in a goroutine and processes events one at a time.
6. `runDeploy` handles the deploy:
   - If `RepoURL` is set: clones or fetches the repository, checks out the exact push commit.
   - Connects to the Uncloud socket and loads the compose file.
   - If `RepoURL` is set and any services have a `build` directive: builds images locally via Docker and pushes them to all cluster machines.
   - Plans and executes the deployment.

## Development

```bash
# Install tools
mise install

# Build binary
mise run build

# Build Docker image
mise run build:image
```

## Testing

Unit tests cover config loading (env vars and YAML), HMAC signature verification, and webhook routing. Run them with:

```bash
go test ./...
```

To test the webhook handler manually without a cluster:

```bash
DEPLOYER_WEBHOOK_SECRET=test mise run run &

curl -X POST http://localhost:8080/webhook \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: sha256=$(echo -n '{"ref":"refs/heads/main","repository":{"full_name":"you/app"},"head_commit":{"id":"abc12345","message":"test"}}' | openssl dgst -sha256 -hmac test | awk '{print $2}')" \
  -H "Content-Type: application/json" \
  -d '{"ref":"refs/heads/main","repository":{"full_name":"you/app"},"head_commit":{"id":"abc12345","message":"test"}}'
```

For YAML config mode, use `/webhook/<name>` instead of `/webhook`.

## Documentation style

Follow the same conventions as `uncloud`:

- Use conversational language, write as if speaking to a colleague.
- Keep sentences simple and optimise for clarity.
- Do not use em dashes or semicolons. Break complex sentences into simpler ones.
- Place the subject before the action. Prefer "The deployer loads the compose file" over "The compose file is loaded by the deployer."
