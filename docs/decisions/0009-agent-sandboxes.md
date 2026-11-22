# 0009: Sandbox presets for agents and distributions, built from pinned images

**Status:** proposed (2026-11-22). **Asked by:** J (review of PR #1): "add sandbox for codex,
claude, opencode, pi, hermes, alpine, ubuntu, kali linux."

## Context

burrow had only small test presets (alpine, curl, trivy). To try burrow as it is meant to be
used, people need sandboxes with an agent inside. Until the preset index exists (#5, #6), these
live with the other test presets: `test/presets/<name>.preset` and
`test/images/<name>/Dockerfile`, built locally by `make sandbox-images`.

Controls served: Secure Baseline Configurations (SCF CFG-04), Least Privilege (IAC-30),
Boundary Protection (NET-03). SCF is credited as in secure-standard-platforms' NOTICE.md.

## What was chosen, and the sources (checked 2026-11-22)

| Preset | Installs | Version (≥ 7 days old) | Licence | Source checked |
|---|---|---|---|---|
| `alpine` | (exists) | alpine 3.22 | | |
| `ubuntu` | base only | `ubuntu:noble-20261024.1` (pushed 2026-10-29) | Ubuntu (various) | Docker Hub tags API |
| `kali` | base only | `kali-rolling` digest of 2026-11-22 | Kali (various) | Docker Hub tags API |
| `claude` | `@anthropic-ai/claude-code` | 2.1.325 (2026-11-13; latest 2.1.330) | proprietary, Anthropic's terms (`LICENSE.md` in the package) | npm registry |
| `codex` | `@openai/codex` | 0.169.0 (2026-11-13; latest 0.171.0) | Apache-2.0 | npm registry |
| `opencode` | `opencode-ai` | 1.19.5 (2026-10-28; latest 1.19.6 is 5 days old) | MIT | npm registry |
| `pi` | `@earendil-works/pi-coding-agent` | 1.5.2 (2026-11-10; latest 1.6.0) | MIT; dependencies MIT, Apache-2.0, BSD, ISC, BlueOak-1.0.0, 0BSD, Unlicense | npm registry; the old `@mariozechner/pi-coding-agent` is deprecated in favour of this name |
| `hermes` | `hermes-agent` (Nous Research) | 0.19.0 (PyPI, 2026-07-20) | MIT | PyPI (author: Nous Research) |

Base images: `node:24.22.3-bookworm` (pushed 2026-11-12) for the npm agents and
`python:3.13.18-bookworm` (2026-11-06) for Hermes, both pinned by digest.

## Options

| Option | For | Against |
|---|---|---|
| **A. Full Debian-based language images (node, python), agent from a lockfile with hashes, no apt** | git, curl and CA certificates are already in the pinned base, so nothing unpinned is installed; every npm/PyPI file is checked by hash; Dependabot updates base, npm and pip | large images (1.3 to 2 GB); the full bases carry many known CVEs |
| B. Slim bases plus `apt-get install git ca-certificates` | smaller | Debian packages are not hash-pinned (whatever the mirror serves that day); D1 |
| C. Vendor install scripts (`curl ... | sh`) | what vendors document | unpinned code from the network at build time; fails D1 outright |
| D. Alpine (musl) bases | smallest | not every agent ships musl binaries; more breakage to chase |

## Decision

**A.**

- **npm agents:** `test/images/<agent>/package.json` names the exact version, and
  `package-lock.json` (generated with `npm install --package-lock-only --before=<7 days ago>`)
  holds every file's integrity hash. `npm ci` installs exactly that. Claude Code and opencode run
  their postinstall script, which copies the native binary for the platform from an optional
  dependency that the lockfile also pins.
- **Hermes:** `requirements.txt` with every dependency's version and SHA-256 (from
  `uv pip compile --generate-hashes --exclude-newer <7 days ago>`), installed with
  `pip --require-hashes --no-deps` into a venv. hermes-agent 0.19.0 (the newest release) pins
  certifi, cryptography, pillow and PyJWT exactly at versions with HIGH/CRITICAL advisories that
  have fixes (Trivy, 2026-11-22: GHSA-6q48-6c7w-9gw2, CVE-2026-69247, CVE-2026-112393,
  CVE-2026-102268 and others). `overrides.txt` (`uv pip compile --override`) raises them to the
  fixed releases, all older than the cooldown (certifi 2026.10.23, cryptography 50.0.2, pillow
  13.0.0, PyJWT 2.15.1; upload dates from PyPI). `hermes --version` starts with them (CI job
  `hermes`); deeper use is *untested*. A line goes once hermes-agent ships the fix.
- **Presets:** `WORKSPACE_MODE=rw`, `NETWORK=proxy`, and an `ALLOW` list with the agent's API and
  login hosts. Those hosts come from the vendors' documentation and are *unverified*: check them
  on first use. A sandbox file's `ALLOW` replaces the list.
- **Credentials:** the user logs in inside the sandbox; the token stays in the sandbox's home
  volume. API keys never go in `ENV`, which is for plain settings. Secrets through the proxy are
  roadmap 0.3.
- Claude Code's auto-updater is off (`DISABLE_AUTOUPDATER=1`): updates come from a new pinned
  image, never from the agent.
- **Scans:** `make scan-sandboxes` and `.github/workflows/sandboxes.yml` (on changes and weekly).
  Secrets fail the scan. Vulnerabilities are reported but do not fail it (see the exception).

## Exceptions (O3)

| What | Owner | Reason | Expiry |
|---|---|---|---|
| `kali` is pinned to a digest younger than 7 days | J | `kali-rolling` has no dated tags and is rebuilt weekly, so no older digest can be named | when the preset index builds its own images (#6), or 2027-05-22 |
| Vulnerabilities in the sandbox images do not fail CI | J | full distribution bases always carry some; the fix is a newer pinned base (Dependabot), which the weekly scan shows | 2027-05-22, or earlier if the preset index brings minimal images |

## Why the others were discarded

- **B** brings unpinned packages into an image whose whole point is pinning.
- **C** runs unpinned remote code at build time.
- **D** trades a size win for platform breakage; revisit when building minimal images for the
  index.

## Consequences and gaps

- The images are big (1.3 to 2 GB). opencode installs every platform's binary (its optional
  packages are not all marked with `os`/`cpu`). Slimming is a task for the preset index.
- The agents' `ALLOW` hosts are unverified; the first real use will show what else they need
  (telemetry, update checks, MCP servers).
- Only `--version` (or `--help`) is checked in CI; logging in and a real task are not tested.
- Claude Code's licence permits use under Anthropic's terms; publishing these images (the index)
  needs a check of the redistribution terms first.
- Kali has no tools beyond the base: the sandbox user has no root, so tools go in a Dockerfile
  `FROM` this image. Raw-socket tools need `NET_RAW`, which only a tool preset can get (`CAP_ADD`).
- Checked locally on Podman 4.9.3: all seven images build (with this environment's TLS proxy CA
  added for the build only) and their agents answer `--version` as uid 1000 with no capabilities.
