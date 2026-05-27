# uncloud-deployer

A continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster, listens for GitHub push webhooks, and deploys your services automatically.

## How it works

When a push lands on your target branch, GitHub sends a webhook to the deployer. The deployer verifies the request and runs the deployment against the cluster through the local Unix socket.

Because the deployer runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

There are two deployment modes:

**Baked-in compose file** — you mount a compose file into the deployer container. On each push the deployer applies that file as-is. Use this when your CI pipeline updates the compose file (for example, by bumping the image tag) before pushing.

**Repo mode** — set `repo_url` (or `DEPLOYER_REPO` in env-var mode) to your repository URL. On each push the deployer clones or fetches the repository at the exact commit, builds any services that have a `build` directive, pushes the images to the cluster, then deploys. Use this when you want the deployer to build images from source.

## Quickstart (single repo, env vars)

**1. Create a compose file for the service you want to deploy.**

Put it at `deploy/compose.yaml` next to your deployer compose file. This is the file the deployer will apply on each push.

**2. Add the deployer to your cluster compose file.**

```yaml
configs:
  app_compose:
    file: ./deploy/compose.yaml

services:
  deployer:
    image: ghcr.io/psviderski/uncloud-deployer:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
    configs:
      - source: app_compose
        target: /deploy/compose.yaml
    environment:
      - DEPLOYER_WEBHOOK_SECRET=${DEPLOYER_WEBHOOK_SECRET}
    ports:
      - target: 8080
        published: 8080
    restart: always
```

**3. Deploy it.**

```bash
DEPLOYER_WEBHOOK_SECRET=<your-secret> uc deploy
```

**4. Configure the GitHub webhook.**

In your repository settings, add a webhook:
- Payload URL: `http://<your-machine-ip>:8080/webhook`
- Content type: `application/json`
- Secret: the same value as `DEPLOYER_WEBHOOK_SECRET`
- Events: send only **push** events

Once the webhook fires on the next push to `main`, the deployer picks it up and runs the deployment.

## Multiple repos (YAML config)

To deploy multiple repositories from a single deployer instance, use a YAML config file instead of environment variables. Each target gets its own webhook path and its own deploy queue.

**1. Create a config file.**

```yaml
# /deploy/config.yaml
socket_path: /run/uncloud/uncloud.sock  # optional, this is the default

targets:
  - name: app
    webhook_secret: <secret-for-app>
    branch: main
    repo_url: https://github.com/you/app
    repo_token: <pat-for-private-repo>  # omit for public repos

  - name: infra
    webhook_secret: <secret-for-infra>
    compose_file: /deploy/infra.yaml    # baked-in mode
```

**2. Mount the config file and set `DEPLOYER_CONFIG`.**

```yaml
configs:
  deployer_config:
    file: ./config.yaml
  infra_compose:
    file: ./infra.yaml

services:
  deployer:
    image: ghcr.io/psviderski/uncloud-deployer:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
      - /var/run/docker.sock:/var/run/docker.sock  # required for repo mode builds
    configs:
      - source: deployer_config
        target: /deploy/config.yaml
      - source: infra_compose
        target: /deploy/infra.yaml
    environment:
      - DEPLOYER_CONFIG=/deploy/config.yaml
    ports:
      - target: 8080
        published: 8080
    restart: always
```

**3. Register a separate webhook in GitHub for each repository.**

Point each webhook to its target path:
- `http://<your-machine-ip>:8080/webhook/app`
- `http://<your-machine-ip>:8080/webhook/infra`

Use the corresponding `webhook_secret` for each.

## YAML config reference

```yaml
listen_addr: :8080                           # optional
socket_path: /run/uncloud/uncloud.sock       # optional

targets:
  - name: <string>                           # required; used in /webhook/<name>
    webhook_secret: <string>                 # recommended
    branch: main                             # default: main
    compose_file: compose.yaml               # default: compose.yaml (repo mode) or /deploy/compose.yaml
    force_recreate: false                    # default: false
    repo_url: https://github.com/org/repo    # enables repo mode
    repo_token: <pat>                        # for private repos; requires Contents: read
    work_dir: /deploy/work/<name>            # default: /deploy/work/<name>
```

## Env-var configuration (single repo)

When `DEPLOYER_CONFIG` is not set, the deployer configures a single target from environment variables and registers it at `/webhook`.

| Variable | Default | Description |
|---|---|---|
| `DEPLOYER_WEBHOOK_SECRET` | _(empty)_ | GitHub webhook secret for HMAC signature verification. Strongly recommended. |
| `DEPLOYER_BRANCH` | `main` | Branch to watch for push events. |
| `DEPLOYER_COMPOSE_FILE` | `/deploy/compose.yaml` or `compose.yaml` | Path to the compose file. Defaults to `compose.yaml` (relative to `DEPLOYER_WORK_DIR`) when `DEPLOYER_REPO` is set. |
| `DEPLOYER_SOCKET_PATH` | `/run/uncloud/uncloud.sock` | Path to the Uncloud daemon Unix socket. |
| `DEPLOYER_LISTEN_ADDR` | `:8080` | Address the webhook HTTP server listens on. |
| `DEPLOYER_FORCE_RECREATE` | `false` | Recreate containers even if the image and config are unchanged. Useful with `:latest` tags. |
| `DEPLOYER_REPO` | _(empty)_ | HTTPS URL of the GitHub repository to clone on each push. Enables repo mode. |
| `DEPLOYER_REPO_TOKEN` | _(empty)_ | GitHub personal access token for private repositories. Requires Contents: read permission. |
| `DEPLOYER_WORK_DIR` | `/deploy/work` | Directory where the repository is cloned in repo mode. |
| `DEPLOYER_CONFIG` | _(empty)_ | Path to a YAML config file. When set, all other env vars are ignored. |

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/webhook` | POST | Webhook receiver in single-target env-var mode |
| `/webhook/<name>` | POST | Webhook receiver for a named target in YAML config mode |
| `/healthz` | GET | Health check, returns `200 ok` |

## Building from source

```bash
docker build -t uncloud-deployer .
```

Or build the binary with mise:

```bash
mise run build
```
