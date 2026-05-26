# uncloud-deployer

A continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster, listens for GitHub push webhooks, and deploys your services automatically.

## How it works

When a push lands on your target branch, GitHub sends a webhook to the deployer. The deployer verifies the request, loads a compose file from a mounted volume, and runs `uc deploy` against the cluster through the local Unix socket.

Because the deployer runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

## Quickstart

**1. Create a compose file for the service you want to deploy.**

Put it at `deploy/compose.yaml` next to your deployer compose file. This is the file the deployer will apply on each push.

**2. Add the deployer to your cluster compose file.**

```yaml
services:
  deployer:
    image: ghcr.io/psviderski/uncloud-deployer:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
      - ./deploy/compose.yaml:/deploy/compose.yaml:ro
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

## Configuration

All configuration is through environment variables.

| Variable | Default | Description |
|---|---|---|
| `DEPLOYER_WEBHOOK_SECRET` | _(empty)_ | GitHub webhook secret for HMAC signature verification. Strongly recommended. |
| `DEPLOYER_BRANCH` | `main` | Branch to watch for push events. |
| `DEPLOYER_COMPOSE_FILE` | `/deploy/compose.yaml` | Path to the compose file to deploy on each push. |
| `DEPLOYER_SOCKET_PATH` | `/run/uncloud/uncloud.sock` | Path to the Uncloud daemon Unix socket. |
| `DEPLOYER_LISTEN_ADDR` | `:8080` | Address the webhook HTTP server listens on. |
| `DEPLOYER_FORCE_RECREATE` | `false` | Recreate containers even if the image and config are unchanged. Useful with `:latest` tags. |

## Updating the deployed app

The compose file at `/deploy/compose.yaml` defines what gets deployed. There are two common patterns.

**Pinned image tags (recommended)**

Your CI pipeline builds the image, tags it with the commit SHA, and updates `compose.yaml` in the repo before pushing. The deployer sees the new file (via a git pull step or a volume update) and deploys the new tag.

**Latest tag**

Set `DEPLOYER_FORCE_RECREATE=true`. The deployer will recreate containers on every push, pulling the latest image. This is simpler but gives up reproducibility.

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/webhook` | POST | GitHub webhook receiver |
| `/healthz` | GET | Health check, returns `200 ok` |

## Building from source

The Dockerfile expects the parent directory of both repos as the build context, because `uncloud-deployer` imports `uncloud` via a local `replace` directive.

```bash
# From the directory containing both uncloud/ and uncloud-deployer/
docker build -f uncloud-deployer/Dockerfile -t uncloud-deployer .
```

Or build the binary with mise:

```bash
cd uncloud-deployer
mise run build
```
