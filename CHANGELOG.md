# Changelog

## 0.1.0 (unreleased): prototype

- burrow runs presets, not images (decision 0005): `burrow up` from a sandbox file (like compose),
  `burrow run PRESET` for an ad-hoc sandbox, `burrow exec PRESET` for tools with workspace and
  proxy inheritance capped by the sandbox, `burrow shell`, `port`, `down`, `ls`, `presets`,
  `--dry-run`.
- Presets are least-privilege siblings (decision 0007): a tool gets what its preset asks for;
  `inherit` with a fallback for when there is no sandbox; `--strict` / `STRICT=true` caps tools by
  their sandbox; `ENV`, `CAP_ADD` (short list), `DESCRIPTION`, several caches scoped per sandbox.
- `burrow presets list`, `presets search TERM`, `presets info PRESET`. No built-in presets: they
  come from the presets folder or a path; test presets with their own Dockerfiles in `test/`.
- Sandbox presets with agents (claude, codex, opencode, pi, hermes) and distributions (ubuntu,
  kali), from pinned bases and lockfiles with hashes; built and scanned weekly (decision 0009).
- burrow checks Podman (4.9+, netavark) before creating anything.
- `--user UID:GID` on `run`, `up`, `exec` and `shell` overrides the preset's `RUN_AS`; `0:0` (root, no capabilities) only from the flag, with a warning (decision 0011).
- Nothing is installed on the host: vet, golangci-lint, tests, govulncheck, fmt and the build run
  in pinned images (`packaging/dev/Dockerfile`); mise, `mise.lock` and its installer are gone
  (decision 0010). The scanner pins moved from `compose.yaml` to
  `packaging/scanners/Dockerfile`.
- iron-proxy 0.56.0 is the default exit: `burrow-proxy` takes one set of flags and starts the
  engine burrow asks for, `-engine iron` (default) or `-engine embedded`, from one image
  (decision 0013). `PROXY` in the sandbox file, `--proxy iron|embedded|bypass` on `up` and `run`,
  `--proxy iron|embedded` on `exec`. `bypass` (no proxy: full internet access and DNS, e.g. for scanners such as sslscan in a Kali sandbox) needs `--accept-risk` and
  prints a warning; tools next to such a sandbox keep their own proxy. e2e checks X1 to X5, M1 to
  M7, O1 to O5.
- Temporary proxy (CONNECT, host allowlist, no private addresses) and a port forwarder in Rust
  (decision 0008); a dedicated egress network per proxy.
- Local-first checks and scans through a Makefile, CI running the same targets, daily Dependabot
  with cooldown and grouping, reviewed by the security team; AGENTS.md (decision 0006). Image
  scanning (Trivy: vulnerabilities and secrets), govulncheck in `make scan`, a daily workflow for
  the mise pin.
