# Development

## Setup

You need **Podman 4.9+ and make**, nothing else: no Go, linters, Rust or mise on the host
([decision 0010](decisions/0010-dev-tools-in-containers.md)). Every check runs in an image pinned
by digest:

- `make vet`, `make lint`, `make test`, `make vuln` build one stage of `packaging/dev/Dockerfile`
  (`scripts/dev.sh`). The source is the build context, not a mount, so this also works from a
  Podman machine's shared folder. vet, lint and tests run with no network; govulncheck reads
  vuln.go.dev.
- `make build` builds the binaries in `packaging/burrow/Dockerfile`; `make forward` checks and
  builds the Rust forwarder in its image; the scanners are pinned containers (`packaging/scanners/Dockerfile`).
- `make fmt` is the one step that mounts the repository (read-write, no network, no capabilities,
  your own user), because it writes the files back.

Then `make hooks`: runs `make check` and `make scan` before every `git push`.

Behind a proxy that intercepts TLS, pass a CA to the builds:
`DEV_BUILD_FLAGS="-v /path/ca.crt:/etc/ssl/certs/ca-certificates.crt:ro" make check` (local only).

Without make (PowerShell), the checks are plain podman commands:

```powershell
foreach ($t in "vet","lint","test","vuln") { podman build --target $t -f packaging/dev/Dockerfile . }
```

Podman 4.9+ is needed for `images` and `e2e` (rootless; on Windows and macOS a Podman machine).
On Windows, run `make` from WSL or Git Bash.

## Building

`make build` (`sh scripts/build-in-container.sh`) builds the binaries for every
platform inside a pinned Go image (`packaging/burrow/Dockerfile`) and copies them to `dist/`.
The source goes in as the build context, which Podman copies into the machine, so this also works
where the shared folder of a Podman machine has no file locking (see WINDOWS.md, "Known issues").
Without `sh` (PowerShell):

```powershell
podman build -f packaging/burrow/Dockerfile -t localhost/burrow-dist:dev .
$c = podman create localhost/burrow-dist:dev /none
podman cp "${c}:/dist" ./dist; podman rm $c
```

The build context has no `.git`, so Go embeds no `vcs.*` stamp (`go version -m`); the version
string is passed in from `git describe`.

## Make targets (the same locally and in CI)

The Makefile owns every command ([decision 0006](decisions/0006-make-agents-md-dependabot.md));
pinned images provide the tools ([decision 0010](decisions/0010-dev-tools-in-containers.md)). `make help` lists them.

| Target | What |
|---|---|
| `make check` | `lint` (go vet, golangci-lint with gosec), `test`, `vuln` (govulncheck), `build`, `forward` (rustfmt, clippy, cargo test, release build of the Rust forwarder in its image) |
| `make scan` | `vuln`, TruffleHog (secrets, git history and tree, no network), Semgrep (SAST), Trivy on the repository (dependencies, misconfigurations, secrets) and on the images we build (`scan-images`: vulnerabilities and secrets), each in a pinned, read-only, unprivileged container; `make scan-secrets` runs one |
| `make fmt` | gofumpt + goimports (golangci-lint image, repository mounted) |
| `make images` | build the proxy and forwarder images |
| `make test-images` | build the images of the test presets (`test/images/<name>/Dockerfile` → `localhost/burrow-test-<name>:dev`) |
| `make e2e` | end-to-end checks on real rootless Podman ([TESTING.md](TESTING.md)) |
| `make build` | static binaries for six platforms in `dist/`, with `SHA256SUMS`, built in a container |
| `make vet` / `make test` / `make vuln` | one check each, in `packaging/dev/Dockerfile` |

## Rules

- **No new Go module or Rust crate** without a decision record (licence, maintenance, size, cooldown). The
  standard library first.
- **Pin everything:** images by digest (with the tag for readers), Actions by commit SHA, tools as
  images in `packaging/dev/Dockerfile`. New versions only after 7 days. Nothing is installed on
  the host or the CI runner.
- **Run podman only through `internal/podman`,** with argument lists. Never a shell, never string
  concatenation into a command.
- Every security property has a test, including a test that tries to break it. A "blocked" check
  passes only when the command ran and failed (R12).
- Commit messages name the requirement or control they serve (R7, SB3, NET-04...).

## Updating pins

| What | How | Automated |
|---|---|---|
| Go modules (govulncheck included), Rust crates, base images and developer tools (`packaging/`, including `packaging/dev`: Go and golangci-lint), test images and their npm/pip lockfiles (`test/images/`), scanner images (`packaging/scanners`), Actions | Dependabot, daily, 7-day cooldown; minor+patch grouped per ecosystem, security updates grouped; reviewed by the security team (CODEOWNERS) | yes |
| Image constants in Go tests (`internal/*/..._test.go`) | edit by hand when the test images change | **no** (issue #7) |

**Security fixes skip the cooldown.** Go modules, crates, images (the Go and golangci-lint images
included) and lockfiles get security updates from Dependabot without the cooldown. By hand: bump
the digest or version, run `make check`, and name the advisory in the commit message. A package
that pins a vulnerable dependency exactly gets an override with the advisory next to it
(`test/images/hermes/overrides.txt`).
