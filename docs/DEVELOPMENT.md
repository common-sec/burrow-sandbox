# Development

## Setup

1. Install [mise](https://mise.jdx.dev) (a release you have checked; CI uses
   `scripts/install-mise.sh`, which verifies the download's SHA-256).
2. `mise trust && mise install`: Go and the linters at the exact versions and checksums in
   `mise.lock`. mise refuses versions younger than 7 days (`minimum_release_age`).
3. `mise run hooks:install`: runs `check` and `scan` before every `git push`.

Podman 4.9+ is needed for `images` and `e2e` (rootless; on Windows and macOS a Podman machine).

## Tasks (the same locally and in CI)

| Task | What |
|---|---|
| `mise run check` | `lint` (go vet, golangci-lint with gosec), `test`, `vuln` (govulncheck), `build` |
| `mise run scan` | TruffleHog (secrets, git history and tree, no network), Semgrep (SAST), Trivy (dependencies, misconfigurations, secrets), each in a pinned, read-only, unprivileged container; `sh scripts/scan.sh secrets` runs one |
| `mise run fmt` | gofumpt + goimports |
| `mise run images` | build the proxy and forwarder images |
| `mise run e2e` | end-to-end checks on real rootless Podman ([TESTING.md](TESTING.md)) |
| `mise run build` | static binaries for six platforms in `dist/`, with `SHA256SUMS` |

## Rules

- **No new Go dependency** without a decision record (licence, maintenance, size, cooldown). The
  standard library first.
- **Pin everything:** images by digest (with the tag for readers), Actions by commit SHA, tools in
  `mise.lock`. New versions only after 7 days.
- **Run podman only through `internal/podman`,** with argument lists. Never a shell, never string
  concatenation into a command.
- Every security property has a test, including a test that tries to break it. A "blocked" check
  passes only when the command ran and failed (R12).
- Commit messages name the requirement or control they serve (R7, SB3, NET-04...).

## Updating pins

| What | How | Automated |
|---|---|---|
| Go modules, base images (`packaging/`), scanner images (`compose.yaml`), Actions | Dependabot PRs, 7-day cooldown | yes |
| Tools in `mise.toml` (Go, golangci-lint, govulncheck) | edit the version, `mise lock`, review the diff | **no**: Dependabot does not read mise.toml (*gap*; Renovate does) |
| mise itself (`scripts/install-mise.sh`) | new version and its SHA-256 from the release's `SHASUMS256.txt` | no |
