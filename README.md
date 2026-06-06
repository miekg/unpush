# unpush

A continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster and deploys your services automatically whenever your branch changes.

## How it works

Because unpush runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

There are two ways to trigger a deploy:

**Webhook** — GitHub sends a push event to unpush. unpush verifies the HMAC signature and triggers a deploy. Use this when your repository is reachable from the internet and you want immediate deploys.

**Poll** — set `poll_interval` on a target and unpush checks the remote branch HEAD via `git ls-remote`, starting immediately when unpush starts and then on the configured interval. When a new commit is detected it triggers a deploy. If the previous deploy failed, it retries on the next check. Use this when you can't expose a public webhook endpoint.

Both triggers can be active on the same target at the same time. Set `enable_webhook: false` to opt out of the webhook endpoint when you only want poll mode.

There are two deployment modes, which can be combined with either trigger:

**Baked-in compose file** — you supply a compose file in the deployer config. On each deploy unpush applies that file as-is. Use this when your CI pipeline updates the compose file (for example, by bumping the image tag) before pushing.

**Repo mode** — set `repo_url` for a target. On each deploy unpush clones or fetches the repository at the exact commit, builds any services that have a `build` directive, pushes the images to the cluster, then deploys. Use this when you want unpush to build images from source.

## Quickstart

**1. Create a config file.**

Webhook trigger:

```yaml
# config.yaml
targets:
  - name: app
    webhook_secret: <your-webhook-secret>
    repo_url: https://github.com/you/app
    repo_token: <pat> # omit for public repos
```

Poll trigger (no webhook needed):

```yaml
# config.yaml
targets:
  - name: app
    poll_interval: 5m
    repo_url: https://github.com/you/app
    repo_token: <pat> # omit for public repos
```

Add as many targets as you need. All targets register a `/webhook/<name>` endpoint by default. Set `enable_webhook: false` to opt out (only makes sense when `poll_interval` is also set).

**2. Add the deployer to your cluster compose file.**

```yaml
configs:
  unpush_config:
    file: ./config.yaml

services:
  unpush:
    image: ghcr.io/tonyo/unpush:latest
    volumes:
      - /run/uncloud/uncloud.sock:/run/uncloud/uncloud.sock
      - /var/run/docker.sock:/var/run/docker.sock # required for repo mode builds
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

**4. Configure the GitHub webhook.** (skip if using `enable_webhook: false`)

In your repository settings, add a webhook:

- Payload URL: `http://<your-machine-ip>:8080/webhook/app`
- Content type: `application/json`
- Secret: the `webhook_secret` you set for that target
- Events: send only **push** events

## Config file reference

The config file defaults to `/deploy/config.yaml`. Set `DEPLOYER_CONFIG` to use a different path. Set `DEPLOYER_STATE_DB` to override the state database path. Set `DEPLOYER_REPO_TOKEN` to set a global GitHub PAT used for any target that does not have its own `repo_token`. Set `LOG_LEVEL` to change the log verbosity (default: `info`; options: `debug`, `info`, `warn`, `error`).

```yaml
listen_addr: :8080                           # optional
socket_path: /run/uncloud/uncloud.sock       # optional
state_db: /deploy/state.db                  # optional; SQLite file recording all deploy attempts

targets:
  - name: <string>                           # required; used in /webhook/<name>
    webhook_secret: <string>                 # required for webhook trigger; strongly recommended
    poll_interval: <duration>                # enables poll trigger, e.g. 5m, 1h
    enable_webhook: true                     # default: true; set false to disable /webhook/<name> (requires poll_interval)
    branch: main                             # default: main
    compose_file: compose.yaml               # default: compose.yaml (repo mode) or /deploy/compose.yaml
    force_recreate: false                    # default: false
    repo_url: https://github.com/org/repo    # enables repo mode; required for poll trigger
    repo_token: <pat>                        # for private repos; requires Contents: read
    work_dir: /deploy/work/<name>            # default: /deploy/work/<name>
```

## Endpoints

| Path              | Method | Description                         |
| ----------------- | ------ | ----------------------------------- |
| `/webhook/<name>` | POST   | Webhook receiver for a named target |
| `/healthz`        | GET    | Health check, returns `200 ok`      |

## Building from source

```bash
docker build -t unpush .
```

Or build the binary with mise:

```bash
mise run build
```
