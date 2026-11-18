# Decisions

One file per decision: context, options considered, decision, why the others were discarded,
consequences. Status: proposed, accepted, superseded. Decisions for the organisation (language,
proxy) live in [secure-standard-platforms](https://github.com/common-sec/secure-standard-platforms);
this folder holds burrow's own.

| # | Decision | Status |
|---|---|---|
| [0001](0001-repository-layout.md) | One Go module; `cmd/<product>`, `internal/`, `packaging/<product>` | proposed |
| [0002](0002-drive-the-podman-cli.md) | Drive the podman CLI now; generate Quadlet/kube YAML later | proposed |
| [0003](0003-definition-format.md) | KEY=VALUE definitions for the prototype | proposed, provisional |
| [0004](0004-licence.md) | Apache-2.0 | proposed |
| [0005](0005-presets-not-images.md) | burrow runs presets, not images: `up` (sandbox file), `run PRESET`, `exec PRESET` with inheritance, `shell` | proposed |
| [0006](0006-make-agents-md-dependabot.md) | Makefile as the entry point; AGENTS.md instead of skills; daily grouped Dependabot reviewed via CODEOWNERS | proposed |
