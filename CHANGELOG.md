# Changelog

## 0.1.0 (unreleased): prototype

- burrow runs presets, not images (decision 0005): `burrow up` from a sandbox file (like compose),
  `burrow run PRESET` for an ad-hoc sandbox, `burrow exec PRESET` for tools with workspace and
  proxy inheritance capped by the sandbox, `burrow shell`, `port`, `down`, `ls`, `presets`,
  `--dry-run`. Built-in presets: alpine, trivy-db, trivy-scan.
- Test stand-in proxy (CONNECT, host allowlist, no private addresses) and port forwarder.
- Local-first checks and scans through a Makefile, CI running the same targets, daily Dependabot
  with cooldown and grouping, reviewed by the security team; AGENTS.md (decision 0006).
