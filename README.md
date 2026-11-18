# burrow

*Part of [CommonSec](https://github.com/common-sec): practical security tools, researched in the open.*

**Sandboxes for AI agents and the tools they run, on rootless Podman.** An agent (Claude Code,
Codex, Copilot) works in a persistent sandbox. When it needs a tool such as Trivy, the tool runs
in *its own* sandbox, next to the agent's, with its own user, folders and network. The only way
out of any of them is a proxy with that sandbox's own allowlist.

> **Status: prototype (0.1).** It runs, it is tested end to end on rootless Podman, and it is meant
> for trying the design, not for protecting anything yet. The proxy is a test stand-in; the real
> one comes from [DR-0009](https://github.com/common-sec/secure-standard-platforms/pull/5). See
> the [roadmap](docs/ROADMAP.md).

## Why

AI amplifies every security gap: agents act with our privileges, and the tools they call are
third-party code too (in March 2026 a compromised Trivy release stole credentials from developer
machines). A tool run with no network but its allowlist, no secrets, a read-only workspace and a
read-only root filesystem has nothing to steal and no way out. burrow makes that the default and
keeps it simple to use.

## How it works

```
             host (or the Podman machine VM on Windows / macOS)
 ┌────────────────────────────────────────────────────────────────────────┐
 │  burrow ──podman CLI──►                                                 │
 │                                                                         │
 │  internal network "burrow-dev" (no route out, no DNS)                   │
 │   ┌──────────────┐        ┌──────────────┐                              │
 │   │ sandbox      │──────► │ proxy        │──► normal network ──► allowed│
 │   │ keep-id user │ CONNECT│ allowlist    │                        hosts │
 │   │ /work (rw)   │        └──────────────┘                              │
 │   └──────────────┘                                                      │
 │                                                                         │
 │  per tool (burrow exec): its own internal network + its own proxy       │
 │   ┌──────────────┐        ┌──────────────┐                              │
 │   │ trivy        │──────► │ proxy        │──► mirror.gcr.io only        │
 │   │ /work (ro)   │        └──────────────┘                              │
 │   └──────────────┘                                                      │
 └────────────────────────────────────────────────────────────────────────┘
```

Details and the threat model: [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Try it

You need Podman 4.9 or newer (on Windows and macOS: a Podman machine), `make` and
[mise](https://mise.jdx.dev).

burrow runs **presets**, not images: a preset is a pinned image plus the settings it is safe to
run with. Some are built in (`burrow presets`); yours go in your presets folder.

```sh
make tools images            # Go and linters from mise.lock; localhost/burrow-proxy:dev and -forward:dev
mise exec -- go build -o burrow ./cmd/burrow

# A sandbox defined by a file, like compose (./burrow.sandbox, or -f):
./burrow up -f examples/burrow.sandbox                      # internal network, proxy, sandbox "dev"
./burrow shell dev -- curl -sI https://pypi.org/simple/     # allowed
./burrow shell dev -- curl -sI https://example.com/         # 403 from the proxy
./burrow exec -s dev trivy-db                               # a tool next to it: own proxy, DB mirror only
./burrow exec -s dev trivy-scan                             # no network, workspace read-only
./burrow port dev 8080                                      # publish a port after start
./burrow down dev                                           # keeps the home volume; --purge removes it

# An ad-hoc sandbox from a preset, on the current folder:
./burrow run alpine --name scratch && ./burrow shell scratch

./burrow --dry-run run alpine                               # print the podman commands only
```

Tools inherit from their sandbox where the preset says `inherit` (workspace mode, allowlist
through their own proxy), never more than the sandbox has. Keys:
[docs/DEFINITIONS.md](docs/DEFINITIONS.md).

Windows: [docs/WINDOWS.md](docs/WINDOWS.md).

## Repository

| Path | What |
|---|---|
| `cmd/burrow` | the CLI (sandbox manager) |
| `cmd/burrow-proxy` | egress proxy, **test stand-in** until the DR-0009 proxy is integrated |
| `cmd/burrow-forward` | port forwarder for running sandboxes |
| `internal/` | shared packages: `config`, `podman`, `sandbox`, `proxy`, `forward` |
| `packaging/<product>/` | how each product is packaged (container images now; releases later) |
| `presets/` | presets built into the binary |
| `examples/` | an example sandbox file |
| `Makefile` | every command, locally and in CI (`make help`) |
| `AGENTS.md` | instructions for coding agents and people (`CLAUDE.md` points to it) |
| `docs/` | [roadmap](docs/ROADMAP.md), [architecture](docs/ARCHITECTURE.md), [development](docs/DEVELOPMENT.md), [definitions](docs/DEFINITIONS.md), [testing](docs/TESTING.md), [decisions](docs/decisions/) |

## Security practices

- **Everything pinned:** tools in `mise.toml` + `mise.lock` (checksums), images by digest, Actions
  by commit SHA, Go modules by `go.sum`. New versions only after a **7-day cooldown** (mise
  `minimum_release_age`, Dependabot `cooldown`).
- **Local first:** `make check` (lint, tests, govulncheck, build) and `make scan`
  (TruffleHog, Semgrep, Trivy in locked-down containers) are what CI runs. `make hooks` runs them
  before every push.
- **Dependabot** every day: minor and patch grouped, security updates grouped, reviewed by the
  security team (CODEOWNERS).
- **No third-party Actions** besides GitHub's checkout; mise is installed from a checksum-verified
  release.
- Reporting vulnerabilities: [SECURITY.md](SECURITY.md).

Design background, requirements and measurements: the
[nested-sandboxing research](https://github.com/common-sec/secure-standard-platforms/tree/main/research/topics/nested-sandboxing)
in secure-standard-platforms.

## Licence

Apache-2.0 (see [LICENSE](LICENSE)), the same as Podman, so code can move upstream
([decision 0004](docs/decisions/0004-licence.md), proposed).
