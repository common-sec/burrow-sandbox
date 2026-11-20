# 0007: Presets are least-privilege siblings; strict mode caps them by the sandbox

**Status:** proposed (2026-11-20). **Asked by:** J (second review of PR #1). **Supersedes** the
"a tool never gets more than its sandbox" rule of [0005](0005-presets-not-images.md).

## Context

0005 capped every tool by its sandbox: no wider workspace mode, the sandbox's allowlist or
narrower, no capabilities. J's review changes the model:

- "Security models is that preset already is already configured to allow least privilege and
  access possible to carry the work that is expected from it. And running it on a sibling
  container instead of nested sandboxes, because independent restrictions can be set on it."
- "For sibling containers it makes sense that some can gain privileges if the specific tool is
  expected have this permissions. For strict sandbox restrictions, some flag and mode should be
  enabled."
- "read-only, no network depends on the preset. If tool requires network, or rw, then it will
  have rw."
- "for inherit values, we are missing the default value... If there is no sandbox what value
  should tool use?"

Controls served (from the organisation's control matrix): Least Privilege (SCF IAC-30), Boundary
Protection (NET-03), Data Flow Enforcement (NET-04), Secure Baseline Configurations (CFG-04). SCF is credited as in secure-standard-platforms' NOTICE.md.

## Options

| Option | For | Against |
|---|---|---|
| **A. Preset decides; `inherit` with a fallback; optional strict mode caps by the sandbox** | matches how tools differ (a scanner needs a DB mirror the agent must not reach; a network tool needs `NET_RAW`); siblings keep restrictions independent; strict mode keeps the old guarantee for those who need it | the preset is trusted by default: a malicious preset in the user's folder runs with what it asks for |
| B. Keep 0005: always capped by the sandbox | one simple guarantee | a tool needing a host the agent must not reach forces that host into the agent's allowlist, which widens the agent: the opposite of least privilege |
| C. Nested presets (Podman in Podman inside the sandbox) | the sandbox's limits apply to every tool by construction | slower (file system access through nested storage), more attack surface (nested Podman), not built; kept for later testing |

## Decision

**A.**

1. **A tool gets what its preset asks for** (workspace mode, network, allowlist, capabilities from
   a short list). The preset's author gives it the least it needs; review of presets is where
   that is checked.
2. **`inherit:FALLBACK`.** `inherit` takes the sandbox's setting; without a sandbox it takes the
   fallback (`WORKSPACE_MODE=inherit:rw`, `NETWORK=inherit:proxy` with the preset's `ALLOW`).
   Plain `inherit` falls back to the narrowest: `ro` and `none`.
3. **Strict mode**: `burrow exec --strict`, or `STRICT=true` in the sandbox file. The tool's
   workspace mode must be no wider than the sandbox's, its allowlist must be covered by the
   sandbox's (same host, or under one of its `*.domain` entries), and `CAP_ADD` is refused. A
   tool that asks for more is refused before anything starts.
4. **`CAP_ADD`** (tool presets only): from Podman's default capability set plus `NET_RAW`
   (`CHOWN`, `DAC_OVERRIDE`, `FOWNER`, `FSETID`, `KILL`, `NET_BIND_SERVICE`, `NET_RAW`,
   `SETFCAP`, `SETGID`, `SETPCAP`, `SETUID`, `SYS_CHROOT`). Never `SYS_ADMIN`, `SYS_PTRACE`,
   `NET_ADMIN` or the like; `no-new-privileges` always stays on; sandboxes never get any.
5. Also in this change: `ENV` (plain settings, `*_PROXY` refused), `DESCRIPTION`, several caches
   per preset scoped to the sandbox (`burrow-cache-<sandbox>_<name>`, `exec` scope without a
   sandbox; `exec` is a reserved sandbox name), and a dedicated egress network per proxy.

## Exception (O3: a relaxed default)

| | |
|---|---|
| What | tools may add capabilities back after `--cap-drop ALL` (`CAP_ADD`), and may get more than their sandbox when strict mode is off |
| Owner | J |
| Reason | least privilege per tool: a sibling's own restrictions are narrower than widening the agent's sandbox to fit the tool |
| Limits | the short `CAP_ADD` list above; `no-new-privileges`; tools never join the sandbox's network; presets never read from the workspace |
| Expiry | review when the preset index exists (signed, reviewed presets) or by 2027-05-20, whichever comes first |

## Why the others were discarded

- **B** forces least privilege to be traded between tools and the agent: whatever one tool needs,
  the agent gets too.
- **C** is not discarded for good: it is the later step for those who want every tool under the
  sandbox's restrictions, once performance is measured (roadmap 0.5).

## Consequences and gaps

- Without strict mode, the presets folder is a trust boundary: whoever can write it decides what
  tools get. Documented in ARCHITECTURE.md and SECURITY.md (weak custom configurations are out of
  scope).
- Caches are scoped per sandbox, not per preset: two presets next to the same sandbox that use
  the same cache name share it. A preset that must not trust what others wrote mounts it `ro`/`O`
  or uses its own name (*gap*: per-preset scoping).
- Only the workspace mode and the allowlist can be inherited today; more (mounts, memory,
  environment) is the "rework of inheritance" item on the roadmap.
- Tests: e2e C1 to C3, S4 to S6, N0 to N2; unit tests in `config` and `sandbox`
  ([FEATURES.md](../FEATURES.md) U9 to U11, U14, U15).
