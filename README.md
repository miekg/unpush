# unpush

An experimental continuous deployment service for [Uncloud](https://github.com/psviderski/uncloud). It runs as a container inside your cluster and deploys your services automatically whenever you push changes to your repository.

## Problem

Let's say you have a repository with your application source code, and you want to build and deploy it whenever you push a change. You could set up a CI pipeline that builds your images and pushes them to a registry, then have a separate process that pulls those images and deploys them to your cluster. But that's a lot of moving parts to maintain, and it can be slow.

`unpush` solves this problem by running inside your Uncloud cluster and deploying your services directly from your Git repository. It can be triggered by GitHub webhooks or by polling the repository for changes. It can also build your images from source if you use the repo mode.

## How it works

Because unpush runs inside the cluster, it has direct access to the Uncloud daemon socket without needing SSH keys or network configuration.

There are two ways to trigger a deploy:

**Webhook** — GitHub sends a push event to unpush. unpush verifies the HMAC signature and triggers a deploy. Use this when your repository is reachable from the internet and you want immediate deploys.

**Poll** — set `poll_interval` on a target and unpush checks the remote branch HEAD via `git ls-remote`, starting immediately when unpush starts and then on the configured interval. When a new commit is detected it triggers a deploy. If the previous deploy failed, it retries on the next check. Use this when you can't expose a public webhook endpoint.

Both triggers can be active on the same target at the same time. Set `enable_webhook: false` to opt out of the webhook endpoint when you only want poll mode.

There are two deployment modes, which can be combined with either trigger:

**Baked-in compose file** — you supply a compose file in the unpush config. On each deploy unpush applies that file as-is. Use this when your CI pipeline updates the compose file (for example, by bumping the image tag) before pushing.

**Repo mode** — set `repo_url` for a target. On each deploy unpush clones or fetches the repository at the exact commit, builds any services that have a `build` directive, pushes the images to the cluster, then deploys. Use this when you want unpush to build images from source.

## Quickstart

**1. Create a config file.**

Option A, webhook trigger:

```yaml
# config.yaml
targets:
  - name: app
    webhook_secret: <your-webhook-secret>
    repo_url: https://github.com/you/app
```

Option B, poll trigger (no webhook needed):

```yaml
# config.yaml
targets:
  - name: app
    poll_interval: 5m
    repo_url: https://github.com/you/app
```

Add as many targets as you need. All targets register a `/webhook/<name>` endpoint by default. Set `enable_webhook: false` to opt out (only makes sense when `poll_interval` is also set).

**2. Add the unpush service to your cluster compose file.**

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

**4. Configure the GitHub webhook.** (skip if using poll trigger only)

In your GitHub repository settings, add a webhook:

- Payload URL: `http://<your-machine-ip>:8080/webhook/app`
- Content type: `application/json`
- Secret: the `webhook_secret` you set for that target
- Events: send only **push** events

## Examples

| Example                                                                  | Trigger | Description                                                                     |
| ------------------------------------------------------------------------ | ------- | ------------------------------------------------------------------------------- |
| [docs/examples/compose-webhook.yaml](docs/examples/compose-webhook.yaml) | Webhook | GitHub sends a push event to unpush. Requires a public endpoint.                |
| [docs/examples/compose-poller.yaml](docs/examples/compose-poller.yaml)   | Poll    | unpush checks the remote branch on a fixed interval. No public endpoint needed. |

## Config file reference

The config file is read from `/deploy/config.yaml` by default.

**Environment variables**

| Variable            | Description                                                          |
| ------------------- | -------------------------------------------------------------------- |
| `UNPUSH_CONFIG`     | Path to the config file. Default: `/deploy/config.yaml`              |
| `UNPUSH_STATE_DB`   | Path to the SQLite state database. Default: `/deploy/data/state.db`  |
| `UNPUSH_REPO_TOKEN` | Global GitHub PAT for targets that don't have their own `repo_token` |
| `LOG_LEVEL`         | Log verbosity: `debug`, `info`, `warn`, `error`. Default: `info`     |

**Top-level fields**

| Field         | Default                     | Description                               |
| ------------- | --------------------------- | ----------------------------------------- |
| `listen_addr` | `:8080`                     | Address the HTTP server listens on        |
| `socket_path` | `/run/uncloud/uncloud.sock` | Path to the Uncloud daemon socket         |
| `state_db`    | `/deploy/data/state.db`     | SQLite file recording all deploy attempts |
| `targets`     | —                           | List of deploy targets (see below)        |

**Target fields**

| Field            | Default                                             | Description                                                                      |
| ---------------- | --------------------------------------------------- | -------------------------------------------------------------------------------- |
| `name`           | required                                            | Unique target name. Used in `/webhook/<name>`                                    |
| `webhook_secret` | —                                                   | HMAC secret for verifying GitHub webhook payloads. Required for webhook trigger  |
| `poll_interval`  | —                                                   | Enables poll trigger. Duration string, e.g. `5m`, `1h`                           |
| `enable_webhook` | `true`                                              | Set `false` to disable the `/webhook/<name>` endpoint. Requires `poll_interval`  |
| `branch`         | `main`                                              | Branch to watch                                                                  |
| `compose_file`   | `compose.yaml` (repo mode) / `/deploy/compose.yaml` | Path to the compose file                                                         |
| `force_recreate` | `false`                                             | Force recreate containers on every deploy                                        |
| `repo_url`       | —                                                   | Enables repo mode. Required for poll trigger. E.g. `https://github.com/org/repo` |
| `repo_token`     | —                                                   | GitHub PAT for private repos. Requires `Contents: read`                          |
| `work_dir`       | `/deploy/work/<name>`                               | Directory where the repository is cloned                                         |

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

## Notes

This project is experimental and has been prototyped using AI assistance.
