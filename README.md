# unpush

A continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster, listens for GitHub push webhooks, and deploys your services automatically.

## How it works

When a push lands on your target branch, GitHub sends a webhook to the deployer. The deployer verifies the request and runs the deployment against the cluster through the local Unix socket.

Because the deployer runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

There are two deployment modes:

**Baked-in compose file** — you supply a compose file in the deployer config. On each push the deployer applies that file as-is. Use this when your CI pipeline updates the compose file (for example, by bumping the image tag) before pushing.

**Repo mode** — set `repo_url` for a target. On each push the deployer clones or fetches the repository at the exact commit, builds any services that have a `build` directive, pushes the images to the cluster, then deploys. Use this when you want the deployer to build images from source.

## Quickstart

**1. Create a config file.**

```yaml
# config.yaml
targets:
  - name: app
    webhook_secret: <your-webhook-secret>
    repo_url: https://github.com/you/app
    repo_token: <pat>   # omit for public repos
```

Each target registers a webhook at `/webhook/<name>`. Add as many targets as you need.

**2. Add the deployer to your cluster compose file.**

```yaml
configs:
  unpush_config:
    file: ./config.yaml

services:
  unpush:
    image: ghcr.io/psviderski/unpush:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
      - /var/run/docker.sock:/var/run/docker.sock   # required for repo mode builds
    configs:
      - source: unpush_config
        target: /deploy/config.yaml
    ports:
      - target: 8080
        published: 8080
    restart: always
```

**3. Deploy it.**

```bash
uc deploy
```

**4. Configure the GitHub webhook.**

In your repository settings, add a webhook:
- Payload URL: `http://<your-machine-ip>:8080/webhook/app`
- Content type: `application/json`
- Secret: the `webhook_secret` you set for that target
- Events: send only **push** events

## Config file reference

The config file defaults to `/deploy/config.yaml`. Set `DEPLOYER_CONFIG` to use a different path.

```yaml
listen_addr: :8080                           # optional
socket_path: /run/uncloud/uncloud.sock       # optional

targets:
  - name: <string>                           # required; used in /webhook/<name>
    webhook_secret: <string>                 # strongly recommended
    branch: main                             # default: main
    compose_file: compose.yaml               # default: compose.yaml (repo mode) or /deploy/compose.yaml
    force_recreate: false                    # default: false
    repo_url: https://github.com/org/repo    # enables repo mode
    repo_token: <pat>                        # for private repos; requires Contents: read
    work_dir: /deploy/work/<name>            # default: /deploy/work/<name>
```

## Endpoints

| Path | Method | Description |
|---|---|---|
| `/webhook/<name>` | POST | Webhook receiver for a named target |
| `/healthz` | GET | Health check, returns `200 ok` |

## Building from source

```bash
docker build -t unpush .
```

Or build the binary with mise:

```bash
mise run build
```
