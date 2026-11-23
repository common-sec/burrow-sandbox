# Security policy

## Reporting a vulnerability

Please **don't open a public issue** for security problems. Report them privately through
GitHub's "Report a vulnerability" (Security → Advisories) on this repository. Include steps to
reproduce and the impact you expect. You'll get an acknowledgement within a few days, and credit
in the release notes if you'd like.

## Scope

burrow is a **prototype**. Only burrow's **defaults** are in scope: a weakness in what burrow does
out of the box, or with the settings its documentation recommends. Misconfigurations and weak
custom configurations (a preset or sandbox file that allows too much, `STRICT=false` where you
needed `true`, a shared folder holding secrets) are **not** in scope; they are the user's choice.
In scope already:
- a way for code in a sandbox or preset to reach a host or address its allowlist does not allow;
- a way for a sandbox to start containers, reach the Podman socket, gain capabilities or become
  root on the host;
- a tool (preset run with `exec`) getting more access than its preset asks for, joining the
  sandbox's network, or, with `--strict`, getting more than its sandbox (wider mounts, a wider
  allowlist, capabilities);
- an agent in a sandbox changing what its next sandbox or tool may do (definition files, presets);
- injection through definition files or names into podman arguments.

Known gaps, out of scope until the roadmap closes them: the temporary proxy does not inspect TLS
(an allowed host is allowed for any path); container escapes through kernel bugs with the crun
engine (use krun, experimental); on Windows, the WSL backend shares a kernel and a network with
other WSL distros (exception E1, see [docs/WINDOWS.md](docs/WINDOWS.md)).

## For auditors

Start with [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (trust boundaries and threat model), then
`internal/sandbox/sandbox.go` (every podman flag burrow sets) and `internal/proxy/proxy.go`.
`scripts/e2e.sh` shows the intended properties as executable checks.
