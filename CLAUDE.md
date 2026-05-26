# CLAUDE.md - uncloud-deployer

This document helps AI assistants understand the codebase and contribute effectively.

## Project overview

`uncloud-deployer` is a continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside an Uncloud cluster and deploys services in response to GitHub push webhooks.

The deployer connects to the Uncloud daemon through its Unix socket (`/run/uncloud/uncloud.sock`), which every node in the cluster exposes. This gives the deployer full cluster access without needing SSH keys or network configuration.

## Repository structure

```
main.go        Entry point. Sets up the HTTP server, handles shutdown.
config.go      Reads all configuration from environment variables.
webhook.go     GitHub webhook handler. Reads body, verifies HMAC, dispatches to deployer.
deployer.go    Core deploy logic. Connects to socket, loads compose file, plans and executes deploy.
Dockerfile     Multi-stage build. Context must be the parent directory of both repos.
compose.yaml   Reference compose file for deploying the deployer itself into Uncloud.
mise.toml      Build tooling. Go 1.26.1 via mise. Tasks: build, run, build:image.
misc/design.md Architecture decisions and options considered during design.
```

## Key dependency

The deployer imports `github.com/psviderski/uncloud` via a `replace` directive pointing to `../uncloud`. This means:

- Both repos must be siblings on disk during development.
- Docker builds must use the parent directory as the build context.
- There is no published module version yet. When uncloud publishes releases, the replace directive should be removed.

The uncloud packages used directly:

| Package | Purpose |
|---|---|
| `pkg/client` | `client.New` creates a cluster client from a connector |
| `pkg/client/connector` | `NewUnixConnector` connects to the daemon socket |
| `pkg/client/compose` | `LoadProject` and `NewDeploymentWithStrategy` implement `uc deploy` logic |
| `pkg/client/deploy` | `RollingStrategy` controls how containers are updated |

## Deploy flow

1. `webhook.go` receives POST `/webhook`.
2. HMAC signature is verified against `DEPLOYER_WEBHOOK_SECRET`.
3. The event is checked: must be a push to the configured branch.
4. `triggerDeploy` sends the event to the deployer's channel (capacity 1). A second concurrent event is queued. A third is dropped with a warning.
5. `deployLoop` runs in a goroutine and processes events one at a time.
6. `runDeploy` opens a fresh connection to the socket for each deploy, loads the compose file, plans the deployment, and executes it.

## Development

```bash
# Install tools
mise install

# Build binary
mise run build

# Build Docker image (run from parent directory)
cd .. && docker build -f uncloud-deployer/Dockerfile -t uncloud-deployer .
```

## Testing

There are no automated tests yet. Manual testing requires a running Uncloud cluster with the socket accessible.

To test the webhook handler locally without a cluster:

```bash
DEPLOYER_WEBHOOK_SECRET=test mise run run &

curl -X POST http://localhost:8080/webhook \
  -H "X-GitHub-Event: push" \
  -H "X-Hub-Signature-256: sha256=$(echo -n '{"ref":"refs/heads/main","repository":{"full_name":"you/app"},"head_commit":{"id":"abc12345","message":"test"}}' | openssl dgst -sha256 -hmac test | awk '{print $2}')" \
  -H "Content-Type: application/json" \
  -d '{"ref":"refs/heads/main","repository":{"full_name":"you/app"},"head_commit":{"id":"abc12345","message":"test"}}'
```

## Documentation style

Follow the same conventions as `uncloud`:

- Use conversational language, write as if speaking to a colleague.
- Keep sentences simple and optimise for clarity.
- Do not use em dashes or semicolons. Break complex sentences into simpler ones.
- Place the subject before the action. Prefer "The deployer loads the compose file" over "The compose file is loaded by the deployer."
