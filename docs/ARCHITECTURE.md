# Architecture

## Components

| Product | Runs where | Does |
|---|---|---|
| `burrow` (CLI) | the user's machine (Linux, Windows, macOS) | reads definitions, checks them, and drives the `podman` CLI. On Windows and macOS `podman` is the remote client of a Podman machine (a Linux VM). burrow never runs inside a sandbox and the sandbox never sees the Podman socket. |
| `burrow-proxy` | a container per sandbox and per tool run | the only exit: HTTPS CONNECT to allowed hosts. A **temporary solution** while a proper proxy is chosen (see the roadmap); it is not extended except for a test or an investigation. |
| `burrow-forward` | a container per published port | relays one port from `127.0.0.1` on the host to one fixed address in a sandbox. Written in Rust, standard library only ([decision 0008](decisions/0008-rust-forwarder.md)). |

**Why a forwarder.** Podman cannot publish a port on a container that is already running: `-p`
only works at `podman run`. An agent often starts a server after the sandbox is up (a dev server,
a test API). The forwarder is the workaround: a small container started next to the sandbox with
`-p 127.0.0.1:H:P`, which relays to the sandbox's address. It is not a sidecar inside the sandbox
and not a proxy for the sandbox's traffic: it only accepts connections from the host and only dials
that one address, so it gives the sandbox no way out.

## Network layout

For a sandbox `dev` (from `burrow up` or `burrow run`):

1. `podman network create --internal --disable-dns burrow-dev`: no gateway, no DNS server.
2. `podman network create burrow-dev-egress`: a normal network with a route out, used **only by
   this sandbox's proxy** (and its port forwarders). Each proxy has its own egress network, so a
   proxy cannot reach another sandbox's proxy or forwarder (on Podman's shared default network it
   could).
3. The proxy starts on `burrow-dev-egress` (its way out), then joins `burrow-dev`. It is the only
   container on both.
4. The sandbox starts on `burrow-dev` only, with `HTTPS_PROXY` pointing at the proxy. A tool that
   ignores the variable has no route at all (fails closed).
5. Each tool run (`burrow exec`) gets the same pattern with its own internal network, its own
   egress network and its own proxy (its own allowlist, or the sandbox's with `NETWORK=inherit`),
   or `--network none`. A tool never joins the sandbox's network.
6. A published port: a forwarder on the sandbox's egress network with `-p 127.0.0.1:H:P`, joined
   to `burrow-dev`, relaying to the sandbox's address. It dials only that address.

## Defaults on every container

`--cap-drop ALL` (a tool preset may add back capabilities from a short list with `CAP_ADD`, see
below), `--security-opt no-new-privileges`, `--http-proxy=false` (podman otherwise copies
the host's proxy variables, credentials included, into the container), a non-root user by default (`RUN_AS`; root only when asked for, with a warning, decision 0011),
`--pids-limit`. Proxy, forwarder and tools also get `--read-only` and a memory limit. The
sandbox maps the host user to its own uid (`--userns keep-id`) so workspace files keep their owner;
tools that mount the workspace do the same with their own uid. A sandbox file inside a read-write
workspace is mounted read-only over itself. Images must be pinned by digest
(or be `localhost/` images built from this repository).

## Security model: presets are least-privilege siblings

([decision 0007](decisions/0007-preset-security-model.md))

- **A preset is already least privilege.** Whoever writes a preset gives it only the access its
  tool needs to do its job: workspace `none`, `ro` or `rw`; no network or an allowlist; the
  capabilities it cannot work without. burrow trusts the preset for that, and the preset's
  author and reviewers are responsible for it.
- **Tools run as siblings, not nested.** A tool runs in its own container next to the sandbox,
  not inside it, so its restrictions are set independently of the sandbox's: a network scanner
  can have `NET_RAW` while the agent's sandbox has no capabilities at all, and a linter can have
  no network while the sandbox has an allowlist.
- **By default a tool gets what its preset asks for**, not capped by the sandbox. `inherit` takes
  the sandbox's setting, or the preset's fallback when there is no sandbox (`inherit:rw`;
  without one, `ro` and `none`).
- **Strict mode** (`burrow exec --strict`, or `STRICT=true` in the sandbox file) caps every tool
  by its sandbox: no wider workspace mode, an allowlist the sandbox's covers, no added
  capabilities. A tool that asks for more is refused before anything starts.
- **Later: nested presets.** Running presets inside the sandbox (Podman in Podman) would put
  every tool under the sandbox's restrictions too, at a cost in performance (mostly file system
  access). To be tested (roadmap).

## Trust boundaries and threat model (short)

Think of the agent in the sandbox as the attacker, and also of a compromised tool, a malicious
definition file and a compromised dependency of burrow.

| Asset | Threat | Control | Gap |
|---|---|---|---|
| Host files, credentials | agent or tool reads them | only `/work` is mounted; no home, SSH agent, socket or host environment | the workspace itself is readable (by design) |
| Network | exfiltration, C2, downloads | internal network; proxy allowlist per sandbox and preset; no IP literals; no private addresses | temporary proxy: an allowed host is allowed for any path; no TLS inspection (until a proxy solution is chosen) |
| Host / Podman | escape, starting containers | rootless Podman, no socket inside, no capabilities, no new privileges | crun shares the kernel (krun for a kernel boundary); on Windows WSL shares a kernel with other distros (E1); the dedicated-distro setup in WINDOWS.md (no automount but one folder, no interop, no sudo) is a temporary protection that root in the distro can undo |
| Other sandboxes and tools | lateral movement | one internal network and one egress network per sandbox and tool run; a tool never joins its sandbox's network | sandboxes of one user share the rootless Podman user namespace |
| The sandbox's limits | a compromised or badly written preset gets more than the sandbox (a wider allowlist, `rw`, capabilities) | presets are least privilege and reviewed; never read from the workspace; `CAP_ADD` only from a short list, never `SYS_ADMIN` and the like; `no-new-privileges` always; `--strict` caps tools by the sandbox | without `--strict`, the preset is trusted: a malicious preset in the user's presets folder runs with what it asks for |
| Definitions | a crafted preset or sandbox file smuggles extra podman options (for example a mount path with a comma that adds `,rw`, or a value starting with `-`) | files are plain data: one KEY=VALUE per line, no variables or includes, unknown keys refused; every value checked before use; podman is called with a list of arguments, never through a shell | – |
| Definitions | the agent edits them to widen its next sandbox or tool | presets are never looked up in the workspace; the sandbox file is mounted read-only inside the sandbox | a preset or sandbox file given by path into the workspace is trusted as the user's choice; files the agent can write elsewhere in the workspace (e.g. a second sandbox file) are not protected |
| burrow's own supply chain | compromised dependency or tool | no Go or Rust dependencies at run time (govulncheck is a pinned dev tool); no tools installed on the host: each runs in an image pinned by digest, the checks with no network; 7-day cooldown; scans before push; daily Dependabot reviewed by the security team | Dependabot does not update image constants in tests; `make fmt` mounts the repository read-write into the pinned golangci-lint image |
| Images burrow starts | a process on the host (for example a sandbox port forwarded to `127.0.0.1:443`) answers as the registry `localhost` and serves its own image for a missing `localhost/` image | `localhost/` images are run with `--pull=never`: a missing one is an error with the build command; other images are pinned by digest and pulled only when missing | a local image already in the store is trusted as built |
| The sandbox's user | a preset or sandbox file asks for root, so the agent (or a shared preset) gets root inside | the defaults are never root; root comes only from `--user`, a preset in the user's presets folder or a sandbox file (never from the workspace implicitly; a sandbox file in the workspace is read-only inside), and every run as root prints a warning naming where it came from (decision 0011); root keeps no capabilities and no-new-privileges, and with rootless Podman it is the host user | root inside can change every file in the container and in an rw workspace; a shared or downloaded preset can ask for root, and only the warning shows it (a verification step for such presets is being evaluated) |

## Podman-native, not a new engine

burrow only adds what Podman lacks (policy, presets, the per-sandbox proxy, ports on running
containers, shims). Everything it does is a plain podman command (`--dry-run` prints them). The
plan is to generate Quadlet or kube YAML later, so Podman owns the lifecycle
([decision 0002](decisions/0002-drive-the-podman-cli.md)).
