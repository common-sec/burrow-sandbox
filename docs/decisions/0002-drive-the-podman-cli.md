# 0002: Drive the podman CLI; generate Podman configuration later

**Status:** proposed (2026-11-18). **Context:** the door must stay open to doing this with
Podman's own features instead of a wrapper, and to move code into Podman later (SB5, W7). burrow
must work on Linux, and on Windows and macOS through a Podman machine (D6, R2).

## Options

| Option | For | Against |
|---|---|---|
| **A. Run the `podman` CLI with argument lists** | works unchanged on Windows and macOS (podman is the remote client of the machine); every action is a command a human can read and replay (`--dry-run`); no Go dependencies | parsing output; slower than an API |
| B. Podman's Go bindings (REST API over the socket) | typed, faster | large dependency tree; needs the socket path per platform; a socket on the host is full control and must never reach a sandbox |
| C. Generate Quadlet files or kube YAML and let Podman/systemd run them | most Podman-native; definitions work without burrow | no systemd on Windows/macOS hosts (it runs in the machine); some features (ports on running containers, per-run proxies) have no Quadlet equivalent yet |
| D. A new engine | – | fails O4 (integrate before building) |

## Decision

**A now, C as the target.** Every burrow action stays expressible as plain podman commands, so it
can be turned into Quadlet or kube YAML once the use cases are clear (roadmap, "Later"). B is not
used: it adds dependencies and a socket for no gain at this stage.

## Consequences

- The `internal/podman` Runner is the only place that runs podman; tests use a recorder.
- Features Podman lacks (a published port on a running container, per-run proxies) are written
  so that they could become Podman features: small, separate, documented.
