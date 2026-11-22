# burrow

*Part of [CommonSec](https://github.com/common-sec): practical security tools, researched in the open.*

**Sandboxes for AI agents and the tools they run, on rootless Podman.** burrow has two kinds of
sandbox, set up in different ways. They complement each other, and each works on its own:

- **A sandbox is a full work environment.** The agent, its tools and everything else can run
  inside it. Its first protections are file system isolation, process isolation, restricted
  network, and (planned) secrets through a proxy.
- **A preset is a standalone tool** with its security restrictions already defined (Trivy, for
  example). It runs in its own container with the least privilege it needs. It can also take
  what it needs from a sandbox.
- **Both can be used together:** presets attached to a sandbox run next to it, so processes,
  secrets and environments stay apart.

Future work still to test: nested sandboxes (Podman in Podman), and an MCP-like server that lets
the agent start subagents and tools under the same restrictions ([roadmap](docs/ROADMAP.md)).

> **Status: prototype (0.1).** It runs, it is tested end to end on rootless Podman, and it is meant
> for trying the design, not for protecting anything yet. The proxy is a temporary stand-in until
> a proxy solution is chosen. See the [roadmap](docs/ROADMAP.md).

## Why

AI amplifies every security gap: agents act with our privileges, and the tools they call are
third-party code too (in March 2026 a compromised Trivy release stole credentials from developer
machines). A tool run with no network but its allowlist, no secrets, a read-only workspace and a
read-only root filesystem has nothing to steal and no way out. burrow makes that the default and
keeps it simple to use.

## Principles

- **The same everywhere.** burrow behaves the same in local development and in CI/CD, and on
  Windows, macOS and Linux. A tool run through burrow gets the same image, settings and
  restrictions wherever it runs.
- **Least privilege per tool.** A preset asks for only what its tool needs: no network or only its
  allowlist, the workspace read-only unless it must write, no secrets.
- **Shimmable tools.** A preset with no default arguments (like `trivy`) can sit behind a shim
  (`trivy` → `burrow exec trivy -- "$@"`), so scripts work the same with and without burrow.
- **Everything pinned**, with a 7-day cooldown for new versions; **local first**: CI runs the same
  `make` targets as a developer.

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

You need only Podman 4.9 or newer (on Windows and macOS: a Podman machine). No Go, mise or make
on your machine: everything is built in pinned containers, from the repository as the build
context, so nothing goes through the machine's shared folder (which has no file locking).

burrow runs **presets**, not images: a preset is a pinned image plus the settings it is safe to
run with. Presets come from your presets folder (`burrow presets` prints it). Published, prebuilt
presets with an index are planned; until then, the test presets in `test/presets` show the format.

```sh
# burrow binaries for every platform, built in the pinned Go image, copied to dist/
podman build -f packaging/burrow/Dockerfile -t localhost/burrow-dist:dev .
c=$(podman create localhost/burrow-dist:dev /none) && podman cp "$c:/dist" ./dist && podman rm "$c"
cp dist/burrow-linux-amd64 ./burrow       # or burrow-darwin-arm64, burrow-windows-amd64.exe ...

# the images burrow starts: proxy, port forwarder, and the test presets' images
podman build -f packaging/proxy/Dockerfile -t localhost/burrow-proxy:dev .
podman build -f packaging/forward/Dockerfile -t localhost/burrow-forward:dev .
for n in alpine curl trivy; do podman build -t "localhost/burrow-test-$n:dev" "test/images/$n"; done
export BURROW_PRESETS_DIR=$PWD/test/presets

./burrow presets list                                       # also: presets search TERM, presets info NAME

# A sandbox defined by a file, like compose (./burrow.sandbox, or -f):
./burrow up -f examples/burrow.sandbox                      # internal network, proxy, sandbox "dev"
./burrow shell dev -- curl -sI https://pypi.org/simple/     # allowed
./burrow shell dev -- curl -sI https://example.com/         # 403 from the proxy
./burrow exec -s dev trivy -- fs /work                      # a tool next to it: own proxy, DB mirrors only
./burrow exec -s dev --strict trivy -- fs /work             # strict: refused, trivy wants hosts dev does not allow
./burrow port dev 8080                                      # publish a port after start
./burrow down dev                                           # keeps the home volume; --purge removes it

# A tool on its own, on the current folder (workspace read-only):
./burrow exec trivy -- fs /work

# An ad-hoc sandbox from a preset, on the current folder:
./burrow run alpine --name scratch && ./burrow shell scratch

./burrow --dry-run run alpine                               # print the podman commands only
```

A tool gets what its preset asks for. Where the preset says `inherit`, it takes the sandbox's
setting (workspace mode, allowlist through its own proxy), or the preset's fallback when there is
no sandbox. With `--strict` (or `STRICT=true` in the sandbox file) a tool never gets more than
its sandbox. Keys: [docs/DEFINITIONS.md](docs/DEFINITIONS.md).

Windows: [docs/WINDOWS.md](docs/WINDOWS.md).

## Repository

| Path | What |
|---|---|
| `cmd/burrow` | the CLI (sandbox manager) |
| `cmd/burrow-proxy` | egress proxy, a **temporary stand-in** until a proxy solution is chosen (not to be extended except for a test or an investigation) |
| `crates/burrow-forward` | port forwarder for running sandboxes (Rust, standard library only) |
| `internal/` | shared packages: `config`, `podman`, `sandbox`, `proxy` |
| `packaging/<product>/` | how each product is packaged (container images now; releases later) |
| `test/presets`, `test/images` | presets for tests and examples, and their images (one Dockerfile each) |
| `examples/` | an example sandbox file |
| `Makefile` | every command, locally and in CI (`make help`) |
| `AGENTS.md` | instructions for coding agents and people (`CLAUDE.md` points to it) |
| `docs/` | [roadmap](docs/ROADMAP.md), [architecture](docs/ARCHITECTURE.md), [development](docs/DEVELOPMENT.md), [definitions](docs/DEFINITIONS.md), [testing](docs/TESTING.md), [decisions](docs/decisions/) |

## Security practices

- **Everything pinned:** tools in `mise.toml` + `mise.lock` (checksums), images by digest, Actions
  by commit SHA, Go modules by `go.sum`. New versions only after a **7-day cooldown** (mise
  `minimum_release_age`, Dependabot `cooldown`).
- **Local first:** `make check` (lint, tests, govulncheck, build, the Rust forwarder) and
  `make scan` (govulncheck, TruffleHog, Semgrep, Trivy on the repository and on the images we
  build, in locked-down containers) are what CI runs. `make hooks` runs them
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
