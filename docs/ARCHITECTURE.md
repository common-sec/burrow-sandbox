# Architecture

## Components

| Product | Runs where | Does |
|---|---|---|
| `burrow` (CLI) | the user's machine (Linux, Windows, macOS) | reads definitions, checks them, and drives the `podman` CLI. On Windows and macOS `podman` is the remote client of a Podman machine (a Linux VM). burrow never runs inside a sandbox and the sandbox never sees the Podman socket. |
| `burrow-proxy` | a container per sandbox and per preset run | the only exit: HTTPS CONNECT to allowed hosts. **Test stand-in** for the DR-0009 proxy. |
| `burrow-forward` | a container per published port | relays one port from `127.0.0.1` on the host to one fixed address in a sandbox. |

## Network layout

For an environment `dev`:

1. `podman network create --internal --disable-dns burrow-dev`: no gateway, no DNS server.
2. The proxy starts on Podman's default network (its way out), then joins `burrow-dev`. It is the
   only container on both.
3. The sandbox starts on `burrow-dev` only, with `HTTPS_PROXY` pointing at the proxy. A tool that
   ignores the variable has no route at all (fails closed).
4. Each preset run gets the same pattern with its own network and its own proxy and allowlist,
   or `--network none`.
5. A published port: a forwarder on the default network with `-p 127.0.0.1:H:P`, joined to
   `burrow-dev`, relaying to the sandbox's address. It dials only that address.

## Defaults on every container

`--cap-drop ALL`, `--security-opt no-new-privileges`, `--http-proxy=false` (podman otherwise copies
the host's proxy variables, credentials included, into the container), a non-root user (`RUN_AS`, never uid 0),
`--pids-limit`. Proxy, forwarder and presets also get `--read-only` and a memory limit. The
sandbox maps the host user to its own uid (`--userns keep-id`) so workspace files keep their owner;
presets that mount the workspace do the same with their own uid. Images must be pinned by digest
(or be `localhost/` images built from this repository).

## Trust boundaries and threat model (short)

| Asset | Threat | Control | Gap |
|---|---|---|---|
| Host files, credentials | agent or tool reads them | only `/work` is mounted; no home, SSH agent, socket or host environment | the workspace itself is readable (by design) |
| Network | exfiltration, C2, downloads | internal network; proxy allowlist per sandbox and preset; no IP literals; no private addresses | stand-in proxy: an allowed host is allowed for any path; no TLS inspection (DR-0009 closes it) |
| Host / Podman | escape, starting containers | rootless Podman, no socket inside, no capabilities, no new privileges | crun shares the kernel (krun for a kernel boundary); on Windows WSL shares a kernel with other distros (E1) |
| Other sandboxes and presets | lateral movement | one internal network each; presets cannot widen the environment's workspace mode | sandboxes of one user share the rootless Podman user namespace |
| Definitions | injection into podman arguments | strict KEY=VALUE format, unknown keys rejected, values validated; arguments passed as a list, never through a shell | – |
| burrow's own supply chain | compromised dependency or tool | no Go dependencies; tools pinned with checksums; 7-day cooldown; scans before push | Dependabot does not update `mise.toml` |

## Podman-native, not a new engine

burrow only adds what Podman lacks (policy, presets, the per-sandbox proxy, ports on running
containers, shims). Everything it does is a plain podman command (`--dry-run` prints them). The
plan is to generate Quadlet or kube YAML later, so Podman owns the lifecycle
([decision 0002](decisions/0002-drive-the-podman-cli.md)).
