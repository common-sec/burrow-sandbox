# Decisions

One file per decision: context, options considered, decision, why the others were discarded,
consequences. Status: proposed, accepted, superseded. Decisions for the organisation (language,
proxy) live in [secure-standard-platforms](https://github.com/common-sec/secure-standard-platforms);
this folder holds burrow's own.

| # | Decision | Status |
|---|---|---|
| [0001](0001-repository-layout.md) | One Go module; `cmd/<product>`, `crates/<product>`, `internal/`, `packaging/<product>`, `test/` | proposed |
| [0002](0002-drive-the-podman-cli.md) | Drive the podman CLI now; generate Quadlet/kube YAML later | proposed |
| [0003](0003-definition-format.md) | KEY=VALUE definitions for the prototype | proposed, provisional |
| [0004](0004-licence.md) | Apache-2.0 | proposed |
| [0005](0005-presets-not-images.md) | burrow runs presets, not images: `up` (sandbox file), `run PRESET`, `exec PRESET` with inheritance, `shell` | proposed; its tool cap superseded by 0007 |
| [0006](0006-make-agents-md-dependabot.md) | Makefile as the entry point; AGENTS.md instead of skills; daily grouped Dependabot reviewed via CODEOWNERS | proposed; mise parts superseded by 0010 |
| [0007](0007-preset-security-model.md) | Presets are least-privilege siblings; `inherit:FALLBACK`; strict mode; `CAP_ADD` short list (exception, owner J) | proposed |
| [0009](0009-agent-sandboxes.md) | Sandbox presets for agents (claude, codex, opencode, pi, hermes) and distributions (ubuntu, kali), from pinned images and lockfiles with hashes | proposed |
| [0008](0008-rust-forwarder.md) | The port forwarder in Rust, standard library only (exception to org DR-0006, owner J) | proposed |
| [0011](0011-secure-defaults-user-overrides.md) | Secure by default, tunable by the user with warnings; root allowed only from `--user` (exception, owner J) | proposed |
| [0010](0010-dev-tools-in-containers.md) | Developer checks in pinned containers (`packaging/dev/Dockerfile`); mise removed; supersedes the mise parts of 0006 | proposed |
| [0012](0012-secrets-through-the-proxy.md) | Secrets through the proxy: random placeholders in the sandbox, substituted at the proxy for the secret's hosts only; values fetched on the host from the user's store (`env:`, `cmd:`; plugins later), never stored by burrow; design in [SECRETS.md](../SECRETS.md) | proposed |
