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
| `make fmt` | before committing Go code (Rust: `cargo fmt` runs as a check in `make forward`) |
| `make check` | before saying a change is done: go vet, golangci-lint (gosec...), tests, govulncheck, build, and the Rust forwarder (rustfmt, clippy, tests) |
| `make scan` | before a push: govulncheck, TruffleHog, Semgrep, Trivy on the repository and on the images we build, in pinned, locked-down containers |
| `make e2e` | when sandbox behaviour changes (needs rootless Podman; builds the test preset images) |
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
- A new feature comes with its user story in [docs/FEATURES.md](docs/FEATURES.md) and the tests
  that prove it (see "Tests" below).
- Update the threat model in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) when a change touches a
  trust boundary. Think of the agent in the sandbox as the attacker, and also of a compromised
  tool, a malicious definition file and a compromised dependency of burrow.

## Supply chain: everything pinned, with a cooldown

- **Pinned to an immutable identifier:** Go modules (`go.sum`), images (digest, with the tag
  kept for readers), GitHub Actions (commit SHA), tools (`mise.lock` with checksums per
  platform), govulncheck (`go.mod` `tool` directive, `go.sum`), Rust crates (`Cargo.lock`),
  mise itself (SHA-256 in `scripts/install-mise.sh`), test preset images (digest in
  `test/images/<name>/Dockerfile`).
- **Cooldown of 7 days** for any new version: mise `minimum_release_age`, Dependabot `cooldown`.
  Security fixes may skip it (`MISE_MINIMUM_RELEASE_AGE=0d`, see docs/DEVELOPMENT.md); say so in
  the commit.
- **Dependabot** checks every day: minor and patch updates come grouped, one PR per ecosystem;
  security updates come grouped too; majors one by one. The security team reviews them through
  `.github/CODEOWNERS`. Dependabot does not read `mise.toml` (update by hand) or
  `scripts/install-mise.sh` (a daily workflow opens a PR) ([docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)).
- **Scan what we ship, not only the code:** every image we build (proxy, forwarder, and later
  presets and releases) is scanned for vulnerabilities and secrets (`make scan-images`), locally
  and in CI. A new artifact is not done until it is in that scan.
- **Generated files are code:** review `mise.lock` and `go.sum` diffs. A changed checksum without
  a version change is a red flag.
- No third-party GitHub Actions besides `actions/checkout`; scanners run as pinned containers.
- No new Go module or Rust crate without a decision record: licence (no AGPL, BSL, NC, ND surprises),
  maintainers (single-maintainer risk), release age, size of its dependency tree. Standard
  library first.

## Security tools

| Tool | What it checks | Where |
|---|---|---|
| golangci-lint (gosec, staticcheck, errcheck, noctx, bodyclose, errorlint...) | code | `make lint` |
| govulncheck | known vulnerabilities in reachable Go code | `make vuln` (also in `make scan`) |
| rustfmt, clippy (`-D warnings`), `cargo test` | the Rust forwarder | `make forward` (in `make check`) |
| TruffleHog | secrets in the tree and the whole git history (no network) | `make scan-secrets` |
| Semgrep | SAST | `make scan-sast` |
| Trivy | dependencies, misconfigurations, secrets in the repository | `make scan-deps` |
| Trivy | vulnerabilities and secrets in the images we build | `make scan-images` |
| `scripts/e2e.sh` | the sandbox's security properties on real Podman, including attempts to break them | `make e2e` |

## Writing Go and Rust in burrow

**Rust (only the forwarder, [decision 0008](docs/decisions/0008-rust-forwarder.md))**
- Standard library only; `#![forbid(unsafe_code)]`; clippy with `-D warnings`.
- The same network rules as Go below: fixed targets, timeouts, a limit on connections, JSON logs.

**Running podman**
- Only through `internal/podman.Runner`, with an argument list. Never `sh -c`, never a command
  built by string concatenation, never user input in the binary name.
- Validate every value before it becomes a podman argument (`internal/config`). Reject what could
  change its meaning: commas, colons and quotes in `-v` paths, leading `-` in names, newlines.
- Defaults on every container: `--cap-drop ALL`, `--security-opt no-new-privileges`,
  `--http-proxy=false`, a non-root `--user`, `--pids-limit`; `--read-only` unless the container
  must write its root filesystem. Never `--privileged`, `--network host` or a Podman/Docker socket.

**Presets and definitions**
- burrow runs presets, not images. A preset is least privilege: it asks only for what its tool
  needs. By default a tool gets what its preset asks for; `inherit` takes the sandbox's setting
  or the preset's fallback; strict mode caps the tool by its sandbox
  ([decision 0007](docs/decisions/0007-preset-security-model.md)).
- `CAP_ADD` only from `config.ToolCaps`; never widen that list without a decision record.
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

Tests start from what a user does. Every feature is written down as a **user story** in
[docs/FEATURES.md](docs/FEATURES.md) ("As a developer, I run a tool next to my sandbox, so that
it cannot write my code"), with the IDs of the tests that prove it. A test that proves no story
is in the wrong place; a story with no test is not done.

What the three kinds of test are for, in plain words:

- **Check every rule with the input that tries to get around it.** For each rule burrow enforces
  (for example "a mount path cannot contain a comma"), one test with a good value, one with a bad
  value, and one with the clever value an attacker would try (`/work,rw`). Written as a table, so
  adding a case is one line. *Goal:* a rule cannot be broken by input nobody thought of.
- **Check the exact podman command.** burrow's security is in the podman flags it sets (no
  capabilities, read-only, which network). Unit tests run burrow against a recorder that captures
  the commands instead of running them, and compare them flag by flag. *Goal:* a change that
  drops or weakens a flag fails a test at once, without needing Podman.
- **Check the real thing, and only count a "blocked" that really happened.** `make e2e` starts real
  sandboxes and tries to break out (another host, a direct IP, the Podman socket, writing the
  workspace). A "blocked" check passes only when the command **ran and failed** (exit 1 to 124),
  never when it could not start: a missing image or a typo must not look like a working
  protection (R12). *Goal:* we know the protection works on a real system, not only that we
  asked for it.

## Pull requests and releases

- Describe the property changed, the tests that prove it, and the gaps left.
- Releases (when they start): reproducible builds, `SHA256SUMS`, an SBOM, and signatures once the
  organisation's signing decision (D7) exists.
- Vulnerability reports come through [SECURITY.md](SECURITY.md): acknowledge, reproduce with a
  test, fix, release, credit.
