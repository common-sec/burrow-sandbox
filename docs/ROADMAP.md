# Roadmap

From the requirements in secure-standard-platforms
([REQUIREMENTS.md](https://github.com/common-sec/secure-standard-platforms/blob/main/research/topics/nested-sandboxing/REQUIREMENTS.md):
principles SB1 to SB7, needed R1 to R12, wanted W1 to W7, exception E1). Each item names the
requirement it serves. One milestone at a time; the order can change after testing on Windows.

Legend: ✓ done · ◐ partly · ○ not started

## 0.1 Prototype: something to play with (this version)

| | Item | Serves |
|---|---|---|
| ✓ | `burrow up` (sandbox file, like compose) and `burrow run PRESET` (ad hoc): persistent sandbox on rootless Podman, internal network, keep-id user, no capabilities, home volume kept across down/up; `shell`, `down`, `ls` | R1, SB2, W1 |
| ✓ | Presets, not images: user presets folder or a path, `--image` / file / preset precedence; `presets list`, `search`, `info` ([0005](decisions/0005-presets-not-images.md)). Built-in presets removed; test presets with one Dockerfile each in `test/` | O7, R8 |
| ✓ | Sandbox file mounted read-only inside the sandbox, so the agent cannot widen it | SB2 |
| ✓ | One proxy per sandbox and per preset run; the proxy is the only exit | SB3, R7 |
| ✓ | `burrow exec PRESET`: tools as ephemeral, least-privilege siblings (own user, workspace none/ro/rw, own proxy with own or inherited allowlist, or no network, cache volumes scoped to the sandbox, overlay caches, `ENV`, `CAP_ADD` from a short list); also without a sandbox ([0007](decisions/0007-preset-security-model.md)) | R3, SB7, W2 |
| ✓ | `inherit` with a fallback for when there is no sandbox (`inherit:rw`, `inherit:proxy`); without one, `ro` and `none` | W2 |
| ✓ | Strict mode (`--strict`, `STRICT=true`): tools capped by their sandbox | SB7 |
| ◐ | Rework of inheritance: which settings a tool can inherit beyond workspace and proxy (caches, extra mounts, memory, environment), each with a default for when there is no sandbox | W2 |
| ○ | Extra volumes and mounts in presets and sandbox files (beyond the workspace and caches) | W2 |
| ○ | Secrets and environment injection: plain `ENV` today; secrets only through the vault/proxy system (placeholders in the sandbox, real values added by the proxy), never as variables | R7, D4 |
| ✓ | `burrow port`: publish a port of a running sandbox through a forwarder container (Rust, standard library only, [0008](decisions/0008-rust-forwarder.md)) | R9 |
| ✓ | A dedicated egress network per proxy: proxies and forwarders of different sandboxes cannot reach each other | SB3 |
| ✓ | Engine per sandbox or preset: crun default, krun experimental (`shell` falls back to a one-shot VM) | R11, W3 |
| ✓ | `--dry-run` prints every podman command | SB5 (reviewable), O7 |
| ✓ | Temporary proxy in Go (CONNECT, host allowlist, ports, no IP literals, no private addresses, JSON audit log) until a proxy solution is chosen; not extended except for tests or investigations | R7 (stand-in) |
| ✓ | End-to-end checks on real rootless Podman, blocked checks only pass when the command ran | R12 |
| ✓ | Base repository: pins, lockfile, cooldown, local-first checks and scans (code and images) through a Makefile, CI, daily grouped Dependabot reviewed by the security team, a daily workflow for the mise pin, AGENTS.md | D1, D2, D5 |
| ○ | Replace `scripts/scan.sh` and `compose.yaml` with burrow itself (scanners as presets, a burrow sandbox deployment) once released | D2, D6 |
| ○ | Tested on Windows with the WSL backend and the Hyper-V backend; differences written down | R2, E1, D6 |

## 0.2 Platforms

| | Item | Serves |
|---|---|---|
| ○ | `docs/WINDOWS.md` filled from real runs: WSL vs Hyper-V (folder sharing, ports, performance, admin rights) | R2, E1 |
| ○ | Hardened WSL machine image: no drive automount, no interop, no root inside, built and pinned in CI | E1 |
| ○ | `burrow doctor`: checks the backend (rootless, machine provider, WSL hardening, SELinux, cgroups) and says what is missing | O7, D5 |
| ○ | Podman machine on Linux too (provider, and the fallback without KVM) | R2 |
| ○ | macOS discovery: try the Podman machine (applehv and libkrun providers), find the security issues (shared folders, ports, what the VM sees) and recommend an install and usage path | D6 |
| ○ | Workspace mounts on SELinux hosts (relabelling) and on machine shares (9p, virtiofs) | R2 |

## 0.3 The real proxy

| | Item | Serves |
|---|---|---|
| ○ | Choose and integrate a proper proxy: TLS inspection, allowlist by host, method and path, credentials injected at the proxy (placeholders in the sandbox), OAuth tokens, audit log | R7 |
| ○ | Secrets and environment injection through the vault/proxy system: which secrets a sandbox or preset may use, how they reach the proxy, how they are rotated | R7, D4 |
| ○ | Later, possibly: one shared proxy with container authentication and a central policy system, instead of one proxy per sandbox | R7, G5 |
| ○ | Per-sandbox CA handed to the sandbox and presets only (certificate, never the key) | R7 |
| ○ | Cost budget per instance (measured: 35 MB, +3 ms per new connection); option to share one policy process | R7, G5 |
| ○ | Retire `burrow-proxy` (or keep it only for tests) | R7 |

## 0.4 Transparent use: shims and presets repository

| | Item | Serves |
|---|---|---|
| ○ | Shims: `trivy` on PATH runs its preset (a preset with no default arguments, like `test/presets/trivy.preset`), so scripts work with and without burrow; policy decides host runs | R5, SB4 |
| ○ | Preset index: prebuilt, published images and an index service to search and download presets (users and CI do not build them); override and configure existing presets, define new ones. Spike on how brew, winget, Debian, Nix and Docker Hub build and index, then a decision record on the architecture and on the repository (here or a dedicated one): issues #5 and #6 | R8, D1 |
| ○ | Presets repository: images built from scratch, signed, pushed to a public OCI registry, usable without burrow | R8, D7 |
| ○ | Decide mise inside the sandbox or only to build preset images | W6 |
| ○ | Host runs only by policy (fail closed) | R3 |
| ○ | Definition format decision (KEY=VALUE today; TOML, YAML, or Podman's own) | 0003 |

## 0.5 Builds and stronger isolation

| | Item | Serves |
|---|---|---|
| ○ | Nested Podman inside the sandbox for builds and tests, never the host socket; also presets run nested (Podman in Podman), under the sandbox's restrictions, measured for performance (file system access) | R4 |
| ○ | krun capabilities tested in full (exec, ports, caches, performance); decide if it leaves experimental | R11 |
| ○ | Pods for sandboxes that share processes, network or files across users | W4 |

## 0.6 Agents

| | Item | Serves |
|---|---|---|
| ○ | MCP-like server that lets the agent run subagents and presets outside the sandbox when allowed, with its own authorisation, proxy and audit log (reviewed like burrow: it adds escape surface) | R6, G6 |
| ○ | Prebuilt sandboxes with AI agents and dev environments, published like presets | W5 |

## Later

| | Item | Serves |
|---|---|---|
| ○ | Generate Quadlet / kube YAML instead of running podman commands, so Podman owns the lifecycle and definitions work without burrow | SB5, decision 0002 |
| ○ | Propose the parts that fit upstream to Podman (port add on running containers, presets) | W7 |
| ○ | Releases: reproducible builds, SBOM (CycloneDX), signatures (once D7 has a solution) | D7 |
