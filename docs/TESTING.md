# Testing

## Unit tests (`make test`)

- `internal/config`: definitions are parsed strictly (unknown keys, duplicates, unpinned images,
  root, IP or wildcard allowlists, paths that would change a `-v` argument are refused); precedence
  preset < sandbox file < flags; tool presets refused as sandboxes; narrowest tool defaults; user
  presets hide built-ins; the built-in presets and the example stay valid.
- `internal/sandbox`: the exact podman arguments: internal network, proxy as the only member with
  a way out, sandbox flags (no capabilities, keep-id, proxy variables, no socket), the sandbox file mounted
  read-only, a running sandbox is not started twice, tools (own network or none, read-only
  workspace, cache, inherit from a sandbox, inherit without one), a tool cannot widen the
  workspace, krun `shell` falls back to a one-shot VM, port forwarder, purge.
- `cmd/burrow`: flags anywhere before `--`; `run`, `exec`, `presets` in `--dry-run`.
- `internal/proxy`: allowlist matching (suffix tricks, IP literals, ports), private addresses
  refused after resolution, CONNECT end to end against a TLS server, plain HTTP refused.
- `internal/forward`: relays a connection.

## End to end (`make e2e`)

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
| E8 | the sandbox writes the workspace (rw) | works |
| E9 | the sandbox edits its own sandbox file (inside the workspace) | blocked (read-only) |
| F1, F2 | publish a port after start; reach it on 127.0.0.1 | works |
| F3 | the forwarder gives the sandbox no way out | blocked |
| P1 | tool (`exec -s`) reads the workspace | works |
| P2 | tool writes the workspace (default ro) | blocked |
| P3 | tool (default `NETWORK=none`) connects out | blocked |
| P4 | tool from the sandbox file's folder (no `-s`) | works |
| I1 | `WORKSPACE_MODE=inherit` on a rw sandbox: tool writes | works |
| I2, I3 | `NETWORK=inherit`: the sandbox's allowed host / another host | works / blocked |
| I4 | inheriting tool reaches the sandbox directly | blocked (no route) |
| S1 | tool without a sandbox reads `--workspace` | works |
| S2, S3 | `inherit` without a sandbox: write / connect out | blocked / blocked |
| R1, R2 | `burrow run alpine` (built-in preset); `shell` into it | works |
| R3 | direct connection from it | blocked (no route) |

### Results

| Date | Where | Podman | Result |
|---|---|---|---|
| 2026-11-18 | Ubuntu 24.04 (cloud agent session), rootless user, runc, cgroups v1 | 4.9.3 | 13/13 (with `E2E_TLS_INSECURE=1`: that network intercepts TLS) |
| 2026-11-18 | same, after the presets redesign (0005) | 4.9.3 | 26/26 (same flag) |
| | Windows, WSL machine | | *to run* |
| | Windows, Hyper-V machine | | *to run* |
