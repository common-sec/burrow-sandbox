# Testing

What each check proves, written as user stories, is in [FEATURES.md](FEATURES.md). This page
lists the checks.

## Unit tests (`make test`)

- `internal/config`: definitions are parsed strictly (unknown keys, duplicates, unpinned images,
  root, IP or wildcard allowlists, paths that would change a `-v` argument are refused); precedence
  preset < sandbox file < flags; tool presets refused as sandboxes; narrowest tool defaults;
  `inherit:FALLBACK` (and bad fallbacks refused); `ENV` (no `*_PROXY`), `CAP_ADD` (only the
  allowed list), `STRICT`, `DESCRIPTION`, several caches; the reserved sandbox name `exec`;
  presets found by name or path, listed and searched; the test presets and the example stay valid.
- `internal/sandbox`: the exact podman arguments: internal network, proxy as the only member with
  a way out, a dedicated egress network per proxy, sandbox flags (no capabilities, keep-id, proxy variables,
  no socket), the sandbox file mounted read-only, a running sandbox is not started twice, tools
  (own network or none, read-only workspace, caches scoped to the sandbox, inherit from a
  sandbox, inherit without one and with a fallback, the preset's own wider settings by default,
  strict mode refusing them, `CAP_ADD`), krun `shell` falls back to a one-shot VM, port
  forwarder on the egress network, purge of the sandbox's own cache volumes.
- `cmd/burrow`: flags anywhere before `--`; `run`, `exec --strict`, `presets list/search/info`
  in `--dry-run`.
- `internal/proxy`: allowlist matching (suffix tricks, IP literals, ports), private addresses
  refused after resolution, CONNECT end to end against a TLS server, plain HTTP refused.
- `crates/burrow-forward` (`cargo test`, run by `make forward` inside the build image): flags,
  only IP:port targets, JSON escaping, relays both ways.

## End to end (`make e2e`)

`scripts/e2e.sh` on real rootless Podman, with the test presets (`test/presets`) and their images
(`make test-images`). A blocked check passes only if the command ran and failed (exit 1 to 124),
never when it could not start.

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
| C1 | by default a tool gets its preset's own allowlist, even a host the sandbox does not allow | works |
| C2 | the same tool with `--strict` | blocked (refused before start) |
| C3 | an inheriting tool with `--strict` | works |
| S1 | tool without a sandbox reads `--workspace` | works |
| S2, S3 | `inherit` without a sandbox and no fallback: write / connect out (default `ro`, `none`) | blocked / blocked |
| S4 | `WORKSPACE_MODE=inherit:rw` without a sandbox: write | works |
| S5, S6 | `NETWORK=inherit:proxy` without a sandbox: the preset's allowed host / another host | works / blocked |
| T1 | the shimmable `trivy` preset takes any arguments | works |
| A1, A2 | a preset with `RUN_AS=0:0`, run without a terminal (`setsid`) / with `CI=true` | blocked (burrow's own refusal, "not accepted") |
| A3 | the same, with `yes` piped into stdin | blocked (the question is asked on the terminal, not stdin) |
| A4 | the same with `--ci --accept root` | works, runs as uid 0 |
| R1, R2 | `burrow run alpine` (preset from the presets folder); `shell` into it | works |
| R3 | direct connection from it | blocked (no route) |
| N0, N1 | from one sandbox's egress network, reach another sandbox's proxy | blocked (separate networks) |
| N2 | the proxy is on its own egress network | works |

### Results

| Date | Where | Podman | Result |
|---|---|---|---|
| 2026-11-18 | Ubuntu 24.04 (cloud agent session), rootless user, runc, cgroups v1 | 4.9.3 | 13/13 (with `E2E_TLS_INSECURE=1`: that network intercepts TLS) |
| 2026-11-18 | same, after the presets redesign (0005) | 4.9.3 | 26/26 (same flag) |
| 2026-11-20 | same, after the security model rework (0007, 0008); forwarder image built without the clippy step (no network in that build) | 4.9.3 | 38/38 (same flag) |
| | Windows, WSL machine | | *to run* |
| | Windows, Hyper-V machine | | *to run* |
