# Working in burrow (for AI agents and people)

burrow is a security tool: it decides what AI agents and their tools can reach. Treat every
change as security-relevant. These instructions are for any coding agent (Claude Code, Codex,
Copilot...) and for people; `CLAUDE.md` only points here.

## Commands

Everything goes through the Makefile, locally and in CI (`make help` lists the targets). mise
provides the pinned tools.

| Command | When |
|---|---|
| `make tools` | once, and after `mise.toml` or `mise.lock` change |
| `make fmt` | before committing Go code |
| `make check` | before saying a change is done: go vet, golangci-lint (gosec...), tests, govulncheck, build |
| `make scan` | before a push: TruffleHog, Semgrep, Trivy in pinned, locked-down containers |
| `make e2e` | when sandbox behaviour changes (needs rootless Podman) |
| `make lock` | after changing a tool version in `mise.toml`; commit `mise.lock` |
| `make hooks` | once: runs `make check scan` before every push |

Report results as they are. If something could not run (no Podman, no network), say so.

## Rules for every change

- Name the requirement the change serves (R1 to R12, SB1 to SB7, W1 to W7, E1: see
  [docs/ROADMAP.md](docs/ROADMAP.md)) in the commit message, and the organisation's controls
  where they apply (SCF IDs such as AAT-39.11, NET-04, TDA-15; SCF is credited as in
  secure-standard-platforms' NOTICE.md).
- New dependencies, formats or design choices need a record in [docs/decisions/](docs/decisions/)
  with the options discarded and why. Verify licences, versions and release dates from primary
  sources, and say when something is unverified.
- Never weaken a default (capabilities, network, mounts, user, pinning, cooldown) to make a test
  pass. Never skip or disable a test. A change that relaxes a default needs an exception (owner,
  reason, expiry) in a decision record.
- Update the threat model in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) when a change touches a
  trust boundary. Think of the agent in the sandbox as the attacker, and also of a compromised
  tool, a malicious definition file and a compromised dependency of burrow.

## Supply chain: everything pinned, with a cooldown

- **Pinned to an immutable identifier:** Go modules (`go.sum`), images (digest, with the tag
  kept for readers), GitHub Actions (commit SHA), tools (`mise.lock` with checksums per
  platform), mise itself (SHA-256 in `scripts/install-mise.sh`), built-in presets (digest).
- **Cooldown of 7 days** for any new version: mise `minimum_release_age`, Dependabot `cooldown`.
  Security fixes may skip it; say so in the commit.
- **Dependabot** checks every day: minor and patch updates come grouped, one PR per ecosystem;
  security updates come grouped too; majors one by one. The security team reviews them through
  `.github/CODEOWNERS`. Dependabot does not read `mise.toml`, the built-in presets or
  `scripts/install-mise.sh`: update those by hand ([docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)).
- **Generated files are code:** review `mise.lock` and `go.sum` diffs. A changed checksum without
  a version change is a red flag.
- No third-party GitHub Actions besides `actions/checkout`; scanners run as pinned containers.
- No new Go module without a decision record: licence (no AGPL, BSL, NC, ND surprises),
  maintainers (single-maintainer risk), release age, size of its dependency tree. Standard
  library first.

## Security tools

| Tool | What it checks | Where |
|---|---|---|
| golangci-lint (gosec, staticcheck, errcheck, noctx, bodyclose, errorlint...) | code | `make lint` |
| govulncheck | known vulnerabilities in reachable code | `make vuln` |
| TruffleHog | secrets in the tree and the whole git history (no network) | `make scan-secrets` |
| Semgrep | SAST | `make scan-sast` |
| Trivy | dependencies, misconfigurations, secrets | `make scan-deps` |
| `scripts/e2e.sh` | the sandbox's security properties on real Podman, including attempts to break them | `make e2e` |

## Writing Go in burrow

**Running podman**
- Only through `internal/podman.Runner`, with an argument list. Never `sh -c`, never a command
  built by string concatenation, never user input in the binary name.
- Validate every value before it becomes a podman argument (`internal/config`). Reject what could
  change its meaning: commas, colons and quotes in `-v` paths, leading `-` in names, newlines.
- Defaults on every container: `--cap-drop ALL`, `--security-opt no-new-privileges`,
  `--http-proxy=false`, a non-root `--user`, `--pids-limit`; `--read-only` unless the container
  must write its root filesystem. Never `--privileged`, `--network host` or a Podman/Docker socket.

**Presets and definitions**
- burrow runs presets, not images. A preset never gets more than its sandbox: `inherit` and
  explicit modes are capped by the sandbox's own.
- Definitions are data: no expansion, no includes, unknown keys are errors.
- Never read presets or settings from the workspace implicitly: the agent can write there. A
  sandbox file inside the workspace is mounted read-only into the sandbox.

**Network code (proxy, forwarder)**
- Deny by default; allowlists match whole labels (`*.example.com` must not match `badexample.com`).
- Resolve names yourself, check every address (no private, loopback, link-local, CGNAT or IP
  literals), then dial the checked address.
- Timeouts on every dial, read and shutdown; contexts on every network call.
- Log decisions as structured JSON. Never log secrets, tokens or bodies.

**Files, errors, concurrency**
- State files 0600, folders 0700; `filepath`, not string joins; Windows paths (`C:\...`) work.
- Wrap errors with context; a `_ =` needs a reason.
- Every goroutine has a way to end; close both sides of a relay.

## Tests

- Table-driven tests for every validation rule, including the input that tries to get around it.
- Sandbox behaviour: assert the exact podman arguments with `podman.Recorder`, and add an e2e
  check for anything a container could exploit. A "blocked" check passes only when the command
  ran and failed (exit 1 to 124), never when it could not start (R12).

## Pull requests and releases

- Describe the property changed, the tests that prove it, and the gaps left.
- Releases (when they start): reproducible builds, `SHA256SUMS`, an SBOM, and signatures once the
  organisation's signing decision (D7) exists.
- Vulnerability reports come through [SECURITY.md](SECURITY.md): acknowledge, reproduce with a
  test, fix, release, credit.
