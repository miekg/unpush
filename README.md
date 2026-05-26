# uncloud-deployer

A continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster, listens for GitHub push webhooks, and deploys your services automatically.

## How it works

When a push lands on your target branch, GitHub sends a webhook to the deployer. The deployer verifies the request and runs the deployment against the cluster through the local Unix socket.

Because the deployer runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

There are two deployment modes:

**Baked-in compose file** — you mount a compose file into the deployer container. On each push the deployer applies that file as-is. Use this when your CI pipeline updates the compose file (for example, by bumping the image tag) before pushing.

**Repo mode** — set `DEPLOYER_REPO` to your repository URL. On each push the deployer clones or fetches the repository at the exact commit, builds any services that have a `build` directive, pushes the images to the cluster, then deploys. Use this when you want the deployer to build images from source.

## Quickstart (baked-in compose file)

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

## Quickstart (repo mode with local builds)

**1. Add the deployer to your cluster compose file.**

```yaml
services:
  deployer:
    image: ghcr.io/psviderski/uncloud-deployer:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - DEPLOYER_WEBHOOK_SECRET=${DEPLOYER_WEBHOOK_SECRET}
      - DEPLOYER_REPO=https://github.com/you/app
      - DEPLOYER_REPO_TOKEN=${DEPLOYER_REPO_TOKEN}   # omit for public repos
    ports:
      - target: 8080
        published: 8080
    restart: always
```

The deployer expects a `compose.yaml` at the root of your repository. Services with a `build` directive are built and pushed to the cluster before deploying.

**2. Deploy it and configure the webhook** as in steps 3 and 4 above.

## Configuration

All configuration is through environment variables.

| Variable | Default | Description |
|---|---|---|
| `DEPLOYER_WEBHOOK_SECRET` | _(empty)_ | GitHub webhook secret for HMAC signature verification. Strongly recommended. |
| `DEPLOYER_BRANCH` | `main` | Branch to watch for push events. |
| `DEPLOYER_COMPOSE_FILE` | `/deploy/compose.yaml` or `compose.yaml` | Path to the compose file. Absolute, or relative to `DEPLOYER_WORK_DIR` when `DEPLOYER_REPO` is set. Default changes to `compose.yaml` when `DEPLOYER_REPO` is set. |
| `DEPLOYER_SOCKET_PATH` | `/run/uncloud/uncloud.sock` | Path to the Uncloud daemon Unix socket. |
| `DEPLOYER_LISTEN_ADDR` | `:8080` | Address the webhook HTTP server listens on. |
| `DEPLOYER_FORCE_RECREATE` | `false` | Recreate containers even if the image and config are unchanged. Useful with `:latest` tags. |
| `DEPLOYER_REPO` | _(empty)_ | HTTPS URL of the GitHub repository to clone on each push. Enables repo mode. |
| `DEPLOYER_REPO_TOKEN` | _(empty)_ | GitHub personal access token for private repositories. Requires Contents: read permission. |
| `DEPLOYER_WORK_DIR` | `/deploy/work` | Directory where the repository is cloned in repo mode. |

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/webhook` | POST | GitHub webhook receiver |
| `/healthz` | GET | Health check, returns `200 ok` |

## Building from source

```bash
docker build -t uncloud-deployer .
```

Or build the binary with mise:

```bash
mise run build
```
