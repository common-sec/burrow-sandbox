# Testing

## Unit tests (`mise run test`)

- `internal/config`: definitions are parsed strictly (unknown keys, duplicates, unpinned images,
  root, IP or wildcard allowlists, paths that would change a `-v` argument are refused); the
  shipped examples stay valid.
- `internal/sandbox`: the exact podman arguments: internal network, proxy as the only member with
  a way out, sandbox flags (no capabilities, keep-id, proxy variables, no socket), presets (own
  network or none, read-only workspace, cache), a preset cannot widen the workspace, krun exec
  falls back to a one-shot VM, port forwarder, purge.
- `internal/proxy`: allowlist matching (suffix tricks, IP literals, ports), private addresses
  refused after resolution, CONNECT end to end against a TLS server, plain HTTP refused.
- `internal/forward`: relays a connection.

## End to end (`mise run e2e`)

`scripts/e2e.sh` on real rootless Podman. A blocked check passes only if the command ran and
failed (exit 1 to 124), never when it could not start.

| ID | Check | Want |
|---|---|---|
| E1 | `burrow up` | works |
| E2 | the sandbox reads the workspace | works |
| E3 | allowed host through the proxy | works |
| E4 | another host | blocked (403) |
| E5 | direct connection, ignoring the proxy | blocked (no route) |
| E6 | write to the root filesystem as the sandbox user | blocked |
| E7 | reach the Podman socket | blocked (not there) |
| F1, F2 | publish a port after start; reach it on 127.0.0.1 | works |
| F3 | the forwarder gives the sandbox no way out | blocked |
| P1 | preset reads the workspace | works |
| P2 | preset writes the workspace (ro) | blocked |
| P3 | preset with `NETWORK=none` connects out | blocked |

### Results

| Date | Where | Podman | Result |
|---|---|---|---|
| 2026-11-18 | Ubuntu 24.04 (cloud agent session), rootless user, runc, cgroups v1 | 4.9.3 | 13/13 (with `E2E_TLS_INSECURE=1`: that network intercepts TLS) |
| | Windows, WSL machine | | *to run* |
| | Windows, Hyper-V machine | | *to run* |
