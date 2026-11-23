# Features as user stories

Each feature is written as a user story: who wants it, what they do, and why. Under each story
are the tests that prove it: end-to-end check IDs from `scripts/e2e.sh` ([TESTING.md](TESTING.md))
and the unit tests (`TestName` in the package named). A story with no test is not done; a test
that proves no story belongs to another story or is not needed.

Who: a **developer** works with an AI agent; a **tool author** writes presets; a **security
reviewer** checks what burrow allows.

## Sandboxes

| # | Story | Proved by |
|---|---|---|
| U1 | As a developer, I start a sandbox for my project from a file in it (`burrow up`), so that the agent works on my code and nothing else of my machine. | E1, E2, E8; `sandbox.TestUpLayout`, `TestUpRefusesRunningSandbox` |
| U2 | As a developer, I let the agent reach only the hosts I list, so that it cannot send my code anywhere else. | E3, E4, E5; `proxy.TestAllowed`, `TestAddrAllowed`, `TestConnectEndToEnd`, `TestOnlyConnect` |
| U3 | As a developer, I know the agent cannot become root (unless I ask for it), start containers or reach Podman, so that it cannot escape to my machine. | E6, E7; `sandbox.TestUpLayout` (`--cap-drop ALL`, no socket) |
| U4 | As a developer, I know the agent cannot edit the sandbox file to widen its own next sandbox. | E9; `sandbox.TestDefinitionReadOnlyInsideWorkspace` |
| U5 | As a developer, I open a port of a sandbox that is already running (a dev server), so that I can reach it from my browser, without giving the sandbox a way out. | F1, F2, F3; `sandbox.TestPortAndDown`; `burrow-forward` cargo tests |
| U6 | As a developer, I start a quick sandbox from a preset on the current folder (`burrow run`). | R1, R2, R3 |

## Tools (presets)

| # | Story | Proved by |
|---|---|---|
| U7 | As a developer, I run a tool next to my sandbox (`burrow exec`), so that it gets only what its preset allows: by default it reads my workspace and has no network. | P1, P2, P3, P4; `sandbox.TestExecPresetNoNetwork`, `TestExecPresetOwnProxyAndCleanup` |
| U8 | As a tool author, I write `inherit` so that the tool takes the sandbox's workspace mode and allowlist, through its own proxy, and never joins the sandbox's network. | I1, I2, I3, I4; `sandbox.TestExecInheritsFromSandbox` |
| U9 | As a tool author, I give a fallback (`inherit:rw`, `inherit:proxy`) so that the tool still works on its own, without a sandbox. Without a fallback it gets read-only and no network. | S1 to S6; `config.TestPresetInheritAndDefaults`, `sandbox.TestExecWithoutSandbox` |
| U10 | As a tool author, I give my tool the least privilege it needs, even something its sandbox does not have (another host, a capability from the short list), because it runs in its own container. | C1; `sandbox.TestExecGetsWhatThePresetAsksStrictCaps`, `config.TestLoadPreset` |
| U11 | As a security reviewer, I turn on strict mode (`--strict`, `STRICT=true`), so that no tool gets more than its sandbox. | C2, C3; `sandbox.TestExecGetsWhatThePresetAsksStrictCaps`, `config.TestSandboxStrictAndEnv`, `main.TestDryRunCommands` |
| U12 | As a developer, I call a tool with any arguments (`burrow exec trivy -- ...`), so that a shim can replace the tool and my scripts work with and without burrow. | T1 |
| U13 | As a developer, I list, search and inspect the presets I have (`burrow presets list/search/info`). | `config.TestFindAndListPresets`, `config.TestTestPresetsAndExample`, `main.TestDryRunCommands` |
| U14 | As a developer, my tools keep their caches between runs, and a tool next to another sandbox cannot read or poison them. | `sandbox.TestExecPresetOwnProxyAndCleanup`, `TestPortAndDown` (purge) |

| U18 | As a developer, I start a sandbox with an agent already in it (`burrow run claude`, or codex, opencode, pi, hermes; or a plain ubuntu or kali), pinned and scanned, so that I do not install the agent on my machine. | `sandboxes.yml` (build, `--version` as uid 1000 with no capabilities, scan); `config.TestTestPresetsAndExample` |
| U19 | As a developer, burrow tells me at once when my Podman is too old or uses the CNI backend, instead of failing inside podman. | `sandbox.TestCheckEngine`, `TestPreflightAsksPodmanInfo` |
| U20 | As a developer, when an image I build locally is missing, burrow stops before creating anything and tells me which `make` target builds it; it never asks a "registry" on localhost for it. | `sandbox.TestImagePullPolicy`, `TestEveryRunSetsPullPolicy`, `TestUpRequiresLocalImages`, `main.TestHintForMissingLocalImage` |
| U21 | As a developer, a `run` or `up` that fails leaves nothing behind, so I can fix the cause and run it again (no "network already exists"). | `sandbox.TestUpCleansUpAfterFailure` |
| U22 | As a developer, I run a sandbox, a tool or a shell as another user (`--user UID:GID` on `run`, `up`, `exec`, `shell`) without editing the preset; root (`0:0`, still without capabilities) can come from the flag, a preset or a sandbox file, and burrow warns every time, naming where it came from. | `sandbox.TestUserFlag`, `sandbox.TestExecRootPreset`, `config.TestNewSandboxUserOverride`, `config.TestRootFromAnySource`, `main.TestRootWarning` |
| U23 | As a developer, I run a tool that reads my code, writes only its output folder and builds in a large `/tmp` (`WORKDIR`, `MOUNT` with `work:`, `tmpfs:`, `volume:`; `--workdir`, `--workspace-mode`, `--mount` override them), and a preset can never mount a folder of my machine outside the workspace: only my sandbox file or a flag can, with a warning. | `config.TestParseMounts`, `TestPresetMounts`, `TestNewSandboxMounts`, `TestResolveMountSources`, `sandbox.TestUpMountsAndWorkdir`, `TestExecMounts`, `TestPurgeRemovesMountVolumes`, `main.TestMountFlags`; e2e M1 to M5 |

## Isolation between sandboxes

| # | Story | Proved by |
|---|---|---|
| U15 | As a security reviewer, I know that one sandbox's proxy and forwarders cannot be reached from another sandbox's networks. | N0, N1, N2; `sandbox.TestUpLayout`, `TestExecPresetOwnProxyAndCleanup` |

## Definitions

| # | Story | Proved by |
|---|---|---|
| U16 | As a security reviewer, I know a crafted preset or sandbox file cannot add podman options (a mount path with a comma, a name starting with `-`, an unpinned image, a malformed user); root is allowed, with a warning (U22). | `config.TestLoadEnvRejects`, `TestLoadPreset` |
| U17 | As a developer, presets are never picked up from my workspace, so that the agent cannot plant one. | `config.TestFindAndListPresets` |
