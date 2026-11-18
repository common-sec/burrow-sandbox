# 0006: Makefile as the entry point, AGENTS.md for instructions, daily grouped Dependabot

**Status:** proposed (2026-11-18). **Asked by:** J (review of PR #1).

## 1. Makefile as the single entry point

**Context.** Commands were mise tasks. J: "add a Makefile to unify structure and make simpler local
and CI pipeline execution."

| Option | For | Against |
|---|---|---|
| **A. Makefile owns the commands; mise only provides pinned tools (`mise exec --`)** | `make` is everywhere developers and CI are; one place for commands; mise keeps pinning and checksums | GNU make is not native on Windows (WSL or Git Bash, as for the e2e already) |
| B. Makefile wrapping `mise run` tasks | small Makefile | two places to read for one command |
| C. Keep mise tasks only | one tool | not what was asked; `mise run` is less familiar than `make` |

**Decision: A.** `mise.toml` keeps `[settings]`, `[tools]` and `[env]`; the tasks moved to the
Makefile. CI calls `make lock-check`, `make tools`, `make check`, `make scan-<kind>`, `make e2e`.
The lock check now uses `git status --porcelain`, so a missing (untracked) `mise.lock` fails too:
`git diff` alone let it pass.

## 2. AGENTS.md instead of Claude skills

**Context.** J: "remove claude skills as looks like they don't have that much of impact. Leave the
common commands and instructions on AGENTS.md and point CLAUDE.md to AGENTS.md. Add security
instructions there."

| Option | For | Against |
|---|---|---|
| **A. AGENTS.md with commands and rules; `CLAUDE.md` is `@AGENTS.md`** | one file every agent reads (the AGENTS.md convention is read by Codex, Copilot, Cursor and others; Claude Code reads it through the import); always in context, not only when a skill triggers | longer always-loaded context |
| B. Keep the skills | loaded only when relevant | Claude-only; J saw little effect |

**Decision: A.** The two skills' rules (secure Go, SSDLC) were folded into AGENTS.md; the
`.claude/skills/` folder is gone.

## 3. Dependabot: daily, grouped, reviewed by the security team

**Context.** J: "all covered by dependabot with auto PRs for security, grouping all minor/patch into
one PR. Run dependabot daily and notify the security team."

| Option | For | Against |
|---|---|---|
| **A. Per ecosystem: `groups` for minor+patch (version updates) and for security updates; daily; reviewers through CODEOWNERS** | documented options (`groups.update-types`, `applies-to`); majors stay separate PRs | up to one grouped PR per ecosystem (4 today), not one in total |
| B. One `multi-ecosystem-groups` group for everything | a single PR | the documentation does not show `update-types` for multi-ecosystem groups, so majors would be mixed in (*unverified*; to test) |
| C. Dependabot `reviewers` option | in the same file | removed on 2025-05-20 in favour of CODEOWNERS ([GitHub changelog](https://github.blog/changelog/2025-04-29-dependabot-reviewers-configuration-option-being-replaced-by-code-owners/)) |

**Decision: A.** Sources: the [Dependabot options reference](https://docs.github.com/en/code-security/reference/supply-chain-security/dependabot-options-reference)
(`update-types: minor, patch, major`; `applies-to: version-updates | security-updates`; "the
cooldown option is only available for version updates, not security updates") and the
[multi-ecosystem guide](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/configuring-multi-ecosystem-updates)
(`patterns` required; no mention of `update-types`).

**Consequences**
- Security update PRs need "Dependabot security updates" switched on in the repository settings
  (or as an organisation default); `dependabot.yml` cannot switch it on.
- `.github/CODEOWNERS` assigns `@common-sec/security` to every file that pins third-party code.
  The team name is *unverified*: it was not visible with the access used to write this; it must
  exist and have write access, or GitHub ignores the line.
- Still not covered: `mise.toml`, built-in presets, `scripts/install-mise.sh` (manual, reviewed by
  the security team). Renovate covers mise; a decision if the manual work grows.
