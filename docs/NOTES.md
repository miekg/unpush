# Notes

## Potential uncloud upstream changes that would simplify this codebase

### 1. Public build API

`internal/cli.BuildServices` does exactly what `buildAndPush` in `build.go` reimplements, but
the `internal/` package is not importable from outside the uncloud module. Moving that logic to
`pkg/build.BuildServices` (or similar) would reduce `build.go` to a single call.

### 2. Container-safe image push

`client.PushImage` uses a local proxy that requires the caller and the Docker daemon to share a
network namespace. That breaks when unpush runs as a container. `build.go` works around it by
using crane to push directly to each machine's unregistry over WireGuard (`machineIP:5000`). If
uncloud offered a `PushImageDirect` (or made `PushImage` detect and handle the cross-namespace
case), the crane dependency and all the push plumbing in `build.go` would go away.

### 3. Machine registry address

`build.go` manually computes each machine's registry address from its subnet:

```go
subnet, err := member.Machine.Network.Subnet.ToPrefix()
machineIP := subnet.Masked().Addr().Next()
registryAddr := net.JoinHostPort(machineIP.String(), strconv.Itoa(unregistryPort))
```

This is fragile: it assumes the machine IP is always the first host in its subnet and hard-codes
port 5000. A `Machine.RegistryAddr() string` method would centralise that knowledge and remove
the dependency on the internal subnet layout.

---

Items 1 and 2 together would let `build.go` collapse into a single
`pkg/build.BuildAndPush(ctx, project, cli)` call, eliminating crane, the temp-file export, the
Docker CLI setup, and the subnet calculation — roughly 150 of the 178 lines in that file.
