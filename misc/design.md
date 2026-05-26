# Design: uncloud-deployer

This document explains the architecture decisions behind `uncloud-deployer` and the options considered during design.

## Goal

Run a continuous deployment service inside an Uncloud cluster. The service receives GitHub push webhooks and deploys services to the cluster automatically, without human intervention.

## How the Uncloud daemon exposes its API

The daemon (`uncloudd`) on each machine exposes its gRPC API two ways.

**Unix sockets (local only)**

- `/run/uncloud/machine.sock` — single-machine API
- `/run/uncloud/uncloud.sock` — proxy socket that fans out requests to all machines in the cluster

Both sockets have mode `0660` owned by `root:uncloud`. A container running as root (Docker's default) can read them without any group membership.

**TCP port 51000 (WireGuard management network only)**

The daemon binds to its WireGuard management IPv6 address on port 51000. An iptables rule restricts access to sources in `fdcc::/16`, which is the WireGuard management network. Containers run on IPv4 subnets and are not WireGuard peers, so they cannot reach this port.

The key insight is that every machine in the cluster runs `uncloudd`, so every machine has both sockets at the same path. A container deployed to any machine can reach the proxy socket without pinning to a specific host.

## Options considered

### Option A: Unix socket mount (chosen)

Mount `/run/uncloud/uncloud.sock` into the deployer container and connect with `connector.NewUnixConnector`.

The proxy socket routes requests to the correct machines automatically. The deployer does not need to know the cluster topology.

**Why this was chosen:**
- Works with zero changes to Uncloud.
- Uses the exact same API surface as the `uc` CLI.
- No SSH keys, tokens, or network configuration needed.
- The proxy socket gives full multi-machine access, not just the local machine.

**Tradeoffs:**
- The socket grants full cluster access. A vulnerability in the deployer could affect the entire cluster. Scoped access tokens would be a better long-term design (see Option D below).
- The container needs to run as root to access the `0660` socket, unless uncloud adds `group_add` support in compose deployments.

### Option B: SSH tunneling

Inject an SSH private key as a secret and have the deployer SSH to the host to tunnel into the socket, the same way the `uc` CLI works from a laptop.

**Why this was not chosen:**
- Requires SSH key management and secure secret injection.
- Needs a stable host address reachable from inside the container.
- Adds operational complexity for no benefit when the socket is directly accessible.

### Option C: Host networking

Run the deployer with `network_mode: host` so it shares the host network stack and can reach the socket or the TCP port directly.

**Why this was not chosen:**
- Eliminates container network isolation.
- The deployer receives webhooks from the internet, making broad network exposure a security concern.

### Option D: New authenticated TCP endpoint in uncloudd

Add a TCP listener to the daemon on the container-facing IPv4 address with token-based gRPC authentication.

**Why this was not chosen (yet):**
- Requires changes to Uncloud core.
- Token provisioning UX needs to be designed.
- The socket mount approach works well enough for now.

This remains the right long-term direction if multiple in-cluster services need programmatic cluster access with different permission scopes.

## Deploy lifecycle

```
GitHub push
    │
    ▼
POST /webhook
    │
    ├── verify HMAC signature
    ├── check event type == "push"
    ├── check ref == "refs/heads/<branch>"
    │
    ▼
triggerDeploy (non-blocking send to channel)
    │
    ├── channel has capacity 1
    ├── second push while first is running: queued
    └── third push while one is queued: dropped, logged as warning
    │
    ▼
deployLoop (single goroutine, processes one deploy at a time)
    │
    ▼
runDeploy
    ├── open fresh connection to /run/uncloud/uncloud.sock
    ├── compose.LoadProject(composeFile)
    ├── compose.NewDeploymentWithStrategy(RollingStrategy)
    ├── deployment.Plan(ctx)
    ├── plan.IsEmpty() → log "up to date", return
    └── plan.Execute(ctx, cli)
```

A fresh connection is opened for each deploy rather than keeping a persistent connection. This avoids stale connection errors if the daemon restarts between deploys.

## Compose file management

The deployer applies a compose file on each push. The file is mounted at `/deploy/compose.yaml` by default.

**Option 1: Pinned tags (recommended)**

The CI pipeline builds the image, tags it with the commit SHA, pushes it to a registry, then commits the updated `compose.yaml` back to the repo. The deployer sees the new file on the next push.

For this to work, the compose file volume must reflect the latest committed version. The simplest approach is to mount a file managed by a git pull step, or to use a shared volume that a separate sync container keeps up to date.

**Option 2: Latest tag with force recreate**

Set `DEPLOYER_FORCE_RECREATE=true`. The deployer forces container recreation on every push, which causes Docker to pull the latest image. Simpler, but loses reproducibility.

## Webhook concurrency model

The deployer uses a single background goroutine for deployments. Deploys run sequentially, not in parallel. This matches `uc deploy` behavior and avoids race conditions when multiple pushes arrive in quick succession.

The channel between the webhook handler and the deploy loop has capacity 1. This acts as a simple queue: at most one deploy can be waiting while another is running. If a third push arrives while the queue is full, the event is dropped. The push is not lost permanently because the next push will trigger a fresh deploy that includes all commits up to that point.

## Security considerations

**Webhook signature verification**

The deployer uses `X-Hub-Signature-256` HMAC-SHA256 verification. Skipping verification (empty `DEPLOYER_WEBHOOK_SECRET`) is allowed but logs a warning. In production, always set the secret.

**Socket access**

The socket grants full cluster control: create, stop, and remove any service, list machines, and more. The deployer runs with this level of access. Future work could introduce scoped tokens in the Uncloud daemon (Option D) to limit the deployer to deploy-only operations.

**No TLS on the webhook endpoint**

The deployer listens on plain HTTP. In production, Caddy (Uncloud's built-in reverse proxy) handles TLS termination. The deployer should be exposed through Caddy rather than directly, so the public endpoint is always HTTPS.
