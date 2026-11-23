# 0013: Confirming weakened settings locally, and a CI mode that never weakens

**Status:** proposed (2026-11-23); option C chosen by J and implemented. **Owner:** J.
**Builds on** [0011](0011-secure-defaults-user-overrides.md) (secure by default, tunable with
warnings) and on its amendment that allows root from presets and sandbox files (`RUN_AS=0:0`).
This record decides how a person confirms such a setting, and what changes when burrow runs in CI.

Serves O3 (deviations are visible exceptions), O7 (usable), SB5 (reviewable), and the
organisation's controls CFG-02 (secure baseline configurations), IAC-30 (least privilege) and
CHG-02 (configuration change control). SCF is credited as in secure-standard-platforms'
NOTICE.md.

## Question

1. Should a local run ask the person to confirm a setting that weakens a default (root today;
   wider `CAP_ADD`, `NETWORK` overrides later), instead of only printing a warning?
2. What is a CI mode: how is it switched on, which defaults change, and how is confirmation
   skipped there without giving an agent a one-variable bypass?

## What a confirmation can and cannot protect against

This decides most of the design, so it comes first.

| Who | Can they reach burrow? | Does a prompt help? |
|---|---|---|
| The agent inside a sandbox | no: no burrow binary, no Podman socket (decision 0011, point 4) | not needed |
| A preset or sandbox file that asks for more than the person expects (shared, from the future index, written by the agent into the workspace and passed by path) | indirectly, when the person runs it | **yes**: this is the main case. The person sees "this preset asks for root" before it starts |
| A script in the repository the person or CI runs (`Makefile`, `scripts/*.sh`, a workflow) that the agent edited | yes, through the person | yes locally, if the script cannot answer the prompt; in CI only code review protects |
| An agent on the host running as the person (Claude Code, Codex outside burrow) | yes, with the person's rights | **only partly**: the agent's own permission prompt shows the burrow command to the person. An agent with a pseudo-terminal can answer a prompt, and it can always run `podman` directly |

So a confirmation is **informed consent against surprises**, not a security boundary against a
host process running as the person. That is a known gap, not a reason to skip it: the case it
covers (a definition asking for more than the person expected) becomes more common once presets
come from an index.

The consequence for CI: **anything that skips the confirmation must not be something an agent
can set cheaply and silently.** The `CI` variable is exactly that: one line in a script or a
`CI=true burrow ...` prefix.

## Prior art

| Tool | What it does | Lesson for burrow |
|---|---|---|
| npm `exec` / `npx` | "When standard input is not a TTY or a CI environment is detected, `--yes` is assumed." ([docs](https://docs.npmjs.com/cli/v11/commands/npm-exec)) | the counter-example: no TTY or `CI` turns the prompt into a yes. An agent never has a TTY, so the prompt protects nobody it should |
| Terraform | `-auto-approve` "Skips interactive approval"; with `-input=false` and no `-auto-approve`, "Terraform will conservatively assume that you do not wish to apply the plan, causing the operation to fail." `TF_IN_AUTOMATION` only adjusts output. ([apply](https://developer.hashicorp.com/terraform/cli/commands/apply), [automation](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform)) | fail closed when nobody can answer; the approval is an explicit flag; the automation variable changes output only |
| OpenSSH | `BatchMode=yes`: "user interaction such as password prompts and host key confirmation requests will be disabled" ([ssh_config](https://man.openbsd.org/ssh_config)); an unknown host key then fails | non-interactive means "fail", never "accept" |
| pnpm | `--frozen-lockfile` "is `true` by default in CI environments" ([docs](https://pnpm.io/cli/install)) | CI detection used to make things **stricter**: spoofing `CI` gains an attacker nothing |
| git | `safe.directory` "is only respected in protected configuration", which "prevents untrusted repositories from tampering with this value" ([docs](https://code.googlesource.com/git/+/HEAD/Documentation/config/safe.adoc)) | the approval must come from a place the repository (the agent) cannot write; same as 0011 point 3 |
| direnv | `direnv allow` "Grants direnv permission to load the given .envrc"; "the security mechanism to avoid loading new files automatically" ([man](https://direnv.net/man/direnv.1.html)) | approval remembered per file; it is re-asked when the file changes (behaviour known from use, *not stated on that page: unverified*) |
| GitHub CLI | `GH_PROMPT_DISABLED` "disable[s] interactive prompting" ([docs](https://cli.github.com/manual/gh_help_environment)); commands then need their flags | turning prompts off ≠ answering them yes |
| Claude Code | refuses `--dangerously-skip-permissions` "when running as root" on Linux and macOS ([docs](https://code.claude.com/docs/en/sandbox-environments)) | a dangerous combination is refused outright; we keep the same idea for `--privileged` and friends (0011 point 4) |
| GitHub Actions | "The GitHub Actions runner now sets the `CI=true` environment variable by default." ([changelog](https://github.blog/changelog/2020-04-15-github-actions-sets-the-ci-environment-variable-to-true/)) | `CI` is a reliable signal that we are in CI, and an unreliable signal that we are *not* being tricked |

## Options

| Option | For | Against |
|---|---|---|
| A. Warning only (today, 0011) | nothing new; scripts and CI just work | a root preset from the index runs as root with a line of stderr nobody reads |
| B. Prompt on a TTY; no TTY or `CI` set skips it (npm's model, the literal reading of the request) | easy to build; CI works unchanged | **fails the threat model**: agents run without a TTY, so the prompt is skipped exactly where it matters, and `CI=true` is a one-word bypass |
| **C. Named acknowledgement, prompt on a TTY, fail closed otherwise; `--ci` and `CI` only tighten** | prompt where a person is; nothing weakened without a word that names it; CI is configured by a reviewed flag in a reviewed file; spoofing `CI` gains nothing | CI configs must list what they accept (`--accept root`); a host agent can still type the flag (but the person sees it in the agent's own permission prompt) |
| D. C, plus remembered approvals (direnv style) | the person is asked once per preset and setting, not every run (O7) | an approval store in the state folder, which a host agent could write; one more file format; invalidation rules to get right |
| E. A policy file outside the user's reach (`/etc/burrow/policy`, deployed by MDM) saying what may be accepted, locally and in CI | the organisation decides, D3/I1; works as a ceiling even against a host agent if the file is root-owned | needs a policy format and MDM deployment; overkill before burrow has users |

## Decision: C now (chosen by J, 2026-11-23); D when the preset index arrives, E later

### Weakening settings get names

Every setting 0011 lets the user weaken gets a short, stable name, starting with `root`
(`RUN_AS` or `--user` with uid 0). Later ones (`cap:NET_RAW`, `network:...`) are added in the
decision that allows them. burrow computes the list for each run before anything starts and
shows it in `--dry-run`.

### How a weakening is accepted

| Source | Accepted? |
|---|---|
| A flag naming it: `--user 0:0` (already explicit, 0011) or the global `--accept root` (before the command: `burrow --accept root run kali`) | yes, no prompt |
| A preset or sandbox file asks for it, a person is at a terminal | prompt on the controlling terminal (`/dev/tty`, `CONIN$` on Windows), never on stdin, so `yes \| burrow ...` does not answer it; the prompt names the setting, the file and where it came from; default **No** |
| A preset or sandbox file asks for it, no terminal (an agent, a script, CI) | **refused**, with the flag to add: `burrow: preset kali asks for root (RUN_AS=0:0); add --accept root to run it` |
| An environment variable (`BURROW_ACCEPT=root`, `CI=true`) | **no**: variables are inherited silently by every child process and are the cheapest thing for an agent to set |

The warning of 0011 is still printed on every run that has an accepted weakening.

### CI mode

- **Switched on by `--ci`**, or automatically when `CI` is set to a true value. Because CI mode
  only makes burrow stricter, it is safe to detect from a variable anyone can set: setting
  `CI=true` to "bypass the checks" makes things harder, not easier. `CI=false` does **not**
  switch it off when `--ci` is given.
- **What it changes** (built):
  - never prompts, never allocates a TTY: a weakening without `--accept` fails (as above);
  - warnings are also printed as a GitHub `::warning::` annotation when `GITHUB_ACTIONS=true`,
    escaped so a file name cannot start a workflow command of its own, so they show in the run
    summary.
- **Considered and not built:** images pinned by digest in CI only (burrow already requires a
  digest or a `localhost/` image everywhere, so there is nothing to tighten); no implicit user
  presets folder in CI (a runner has none, and burrow's own CI sets `BURROW_PRESETS_DIR`
  explicitly); a JSON warning line and `--warnings-as-errors` (wait for the audit log, roadmap).
- **What it never changes:** the defaults themselves, strict mode, the things 0011 keeps
  impossible. No "CI profile" loosens anything.
- **How verification is skipped safely in CI:** it is not skipped, it is pre-answered. The
  workflow file says `burrow --ci --accept root run kali ...`; that file is code-reviewed
  (CODEOWNERS on `.github/`), so a new weakening shows in a diff with its name in it.

### Why not B, as first suggested

B gives the person a prompt and gives everyone else a free pass: an agent running burrow has no
TTY, and any script can export `CI=true`. The person would see prompts in their own terminal
and believe the setting is guarded, while the paths an attacker would use skip it. C keeps the
prompt for people and turns "nobody can answer" into "no".

### Remembered approvals (D, later)

When presets come from an index, asking every run is too much (O7). Then: remember an approval
in the state folder (0600), keyed by the preset's content hash and the set of weakening names;
a changed preset or a new weakening asks again. Not before the index: today presets live in
the person's own folder and the prompt is rare.

## Exception (O3)

None new. C adds a check; it relaxes nothing. The root exception stays in 0011.

## Implementation

`internal/consent` (names, `--accept` parsing, the question on the controlling terminal, the
refusal that names the flag, `CI` detection); `cmd/burrow` checks root before `up`, `run`,
`exec` and `shell` start anything, then warns as before. `--dry-run` neither asks nor refuses: it
prints what a real run would need. The question goes to `/dev/tty` (`CONIN$`/`CONOUT$` on
Windows) and control characters in file names are replaced, so a crafted path cannot rewrite it.

Tests: `consent.TestCheck` (every way to get a weakening through, as a table),
`TestQuestionNamesTheSource`, `TestQuestionHasNoControlCharacters`, `TestParseAccept`,
`TestDryRunReportsWithoutAsking`, `TestCIFromEnv`; `main.TestRootNeedsAcceptance`,
`TestCIModeOnlyTightens`, `TestAnnotationEscapes`; e2e A1 to A4 (FEATURES U23, U24).

## Gaps

- An agent on the host, running as the person, can type `--accept root`, answer a prompt
  through a pseudo-terminal, or run `podman` directly. burrow cannot stop a process with the
  person's rights; the named flag only makes the weakening visible in the agent's own
  permission prompt. Option E (a root-owned policy) is the answer if this matters.
- In CI, only code review protects the workflow file. A workflow the agent edits and nobody
  reviews runs what it says.
- The CI detection list is only `CI`; other systems (Jenkins sets `JENKINS_URL`, not `CI`:
  *unverified*) need `--ci`.
- Windows console prompt (`CONIN$`) not tested.
- Root in a stored sandbox is checked at `up`/`run`; a later `burrow shell` into it runs as the
  stored user without asking again (it was accepted when the sandbox started).
- Nothing records accepted weakenings yet (audit log: roadmap), same gap as 0011.

## Why the others were discarded

- **A:** the warning alone does not stop a preset from the index silently running as root.
- **B:** skips the check for agents and for anyone who sets `CI`; see above.
- **D now:** an approval store solves a problem we do not have yet (rare prompts), and adds a
  file the agent could write.
- **E now:** needs a policy format and MDM deployment before burrow has users; kept as the
  answer to the host-agent gap.
