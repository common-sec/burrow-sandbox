# 0010: Developer checks in pinned containers; mise removed

**Status:** proposed (2026-11-22). **Asked by:** J (review of PR #1): "mise installation makes no
sense, as we are trying to prevent installing tooling on host in order to protect", and on the
README: "We need to replace it by burrow, or direct podman commands instead." Supersedes the mise
parts of [0006](0006-make-agents-md-dependabot.md) (section 1) and of [0008](0008-rust-forwarder.md).

Serves D1 (pinned), D2 (every step in a sandbox), D6 (same on every platform), O7 (simple), and
the organisation's controls TDA-15 (developer tools), CFG-04 (software use restrictions) and
CHG-02 (configuration change control). SCF is credited as in secure-standard-platforms' NOTICE.md.

## Context

Until now a developer installed mise on the host, and mise installed Go and golangci-lint there
(`mise.lock`, checksums, 7-day `minimum_release_age`). That puts third-party tools on the host
with the developer's rights, which is what burrow exists to avoid. On Windows it also failed:
the Go toolchain cannot run from a Podman machine's shared folder (no file locking, see
[WINDOWS.md](../WINDOWS.md)). The Rust forwarder (0008) and the scanners already ran only in
pinned containers.

## Options

| Option | For | Against |
|---|---|---|
| **A. Each check is a stage of `packaging/dev/Dockerfile`, run with `podman build --target`** | only Podman (and make) on the host; the source is the build context, so it works from a share without locks; `RUN --network=none` for vet, lint and tests; Dependabot updates the images (closes the old `mise.toml` gap) | a build per check (cached layers keep it short); `make fmt` still needs a mount to write files back |
| B. Keep mise on the host | worked; checksums and cooldown per tool | tools on the host (J's point); failed on Windows |
| C. `podman run` with the repository mounted | one image, simple | the share has no file locking (fails on Podman machines); every tool gets write access to the repository |
| D. Run the checks with burrow itself (`burrow exec`, one preset per tool) | the end goal: we would use what we build | burrow must be built first, and its workspace is a mount, so C's lock problem stays until workspaces can live in a volume (roadmap 0.2) |
| E. A dev container (`devcontainer.json`) | editor integration | needs the devcontainer CLI (Node) or VS Code on the host; a second way of working |

**Decision: A**, with D as the target once workspaces can live in a volume.

- `make vet`, `make lint`, `make test`: `podman build --target vet|lint|test`; the module download
  is its own step (checked by `go.sum` and `go mod verify`), the checks run with no network.
- `make vuln`: the `vuln` stage, with network (govulncheck reads vuln.go.dev). A build argument
  with the date makes a cached result at most one day old.
- `make build`: the existing `packaging/burrow/Dockerfile` (`scripts/build-in-container.sh`).
- `make fmt`: `podman run` of the pinned golangci-lint image with the repository mounted
  read-write, no network, no capabilities, read-only root, as the developer's own user. The only
  step that writes the working tree.
- `make e2e` uses the binary from `make build`, so the host needs no Go.
- `mise.toml`, `mise.lock`, `scripts/install-mise.sh`, `scripts/update-mise.sh` and the daily
  `update-pins.yml` workflow are removed. `.containerignore` keeps `.git`, `.cache`, `dist` and
  Rust's `target` out of every build context.

## Sources (checked 2026-11-22)

| What | Version | Where |
|---|---|---|
| Go image | `golang:1.27.4-alpine`, digest in the Dockerfile (same as packaging/burrow) | Docker Hub, `library/golang` |
| golangci-lint image | `golangci/golangci-lint:v2.15.0-alpine`, `sha256:2ba875b9...`, pushed 2026-10-16; `golangci-lint --version`: built with go1.27.0; its Go is 1.27.2 | Docker Hub, `golangci/golangci-lint`; run locally |
| golangci-lint licence | GPL-3.0, used as a tool, not linked into burrow | github.com/golangci/golangci-lint `LICENSE` |
| `RUN --network=none` | supported by `podman build` 4.9.3 (tested locally) | Buildah |

## Consequences and gaps

- **Cooldown:** Dependabot's 7-day `cooldown` on the docker ecosystem replaces mise's
  `minimum_release_age`. A hand edit of a digest bypasses it; CODEOWNERS sends every change under
  `packaging/` and `scripts/` to the security team. Security fixes: Dependabot security updates,
  or a hand bump that names the advisory in the commit.
- **Lint runs on Go 1.27.2** (the golangci-lint image), vet, tests and builds on 1.27.4. Both
  satisfy `go 1.27`; `GOTOOLCHAIN=local` stops either from downloading another toolchain.
- **Two digests of the Go image** (packaging/burrow, packaging/dev, and packaging/proxy) must be
  updated together; Dependabot proposes them in the same grouped PR (*to confirm on its first run*).
  Whether Dependabot updates an image named in an `ARG` default (packaging/burrow, packaging/proxy)
  is *unverified*; packaging/dev uses plain `FROM` lines.
- **make** is still needed on the host for the targets. PowerShell users run the same
  `podman build --target ...` lines by hand (DEVELOPMENT.md).
- **Network during builds:** the module download and govulncheck need it; behind a proxy that
  intercepts TLS, `DEV_BUILD_FLAGS` passes extra flags (a CA) to `podman build`. Local only.
- No exception needed: no default is relaxed. Pinning moves from `mise.lock` to image digests.

## Why the others were discarded

- **B:** fails the point of the project (tools on the host) and failed on J's Windows machine.
- **C:** fails on Podman machine shares, and gives every tool write access to the code.
- **D:** blocked by the same lock problem until workspaces live in a volume; kept as the target.
- **E:** puts Node or an editor on the host as a requirement, and adds a second way of working.
