# Decisions

Why things are the way they are, for the next person (or agent) who wonders. One entry per
decision: what, why, what was rejected. Measured results that closed a question are in
`recommendations.md`; behavior is in `architecture.md`.

## Changing agents' configs

**Nothing changes without an answer.** Anything beyond gascoigne's own files (allow rules, updating
installed hooks, setting up a newly found agent) is a *question* (`decision` in `setup.go`), asked in
a dashboard page or `gascoigne setup`. Found things are *signaled* (a one-line session note, once),
never acted on. Rejected: silent auto-update by default, installing every agent found on PATH.

**Autosync is opt-in and user-level only.** Hooks may rewrite user-scope installs after an upgrade
when the user said `on`. Project files sit in repositories (often committed), so only a command the
user runs (`gascoigne upgrade`) touches them. Not decided means nothing is synced.

**Claude's automatic update writes `settings.json` only.** `~/.claude.json` is rewritten by a
running Claude Code; writing it from a hook can lose data. Registration there stays with `install`.

**`registrationVersion`** (`installs.go`) numbers what `install` writes. Bump it whenever hooks,
matchers, guide text, rules or commands change; `registrationNotes` says what changed. Installs
record the number they were written with (`installs.json`), so "stale" is a comparison, not a guess.

**Installs from before the manifest are found, not forgotten.** The first release recorded nothing,
so "a new version was installed" cannot depend on `installs.json`. The agent's own config (hook
present) is the evidence; such installs count as registration 0 and trigger the same question.

**One upgrade command.** `gascoigne upgrade` gathers what a new version needs: hooks and rules, the
user's `config.yaml` (via the existing `config -upgrade` code), open choices. `doctor` only reports.
Removed before release: `install -sync`, `doctor -fix` (two ways to do one thing).

**An unanswered page comes back.** Closing the page without saving answers nothing, so the next
session start opens it again (not on compact or /clear; at most every ten minutes, so quick restarts do
not open a window each time). Answering, even "not now" or "no", ends it. Rejected: asking once per
version (a closed tab would mean the update never happens).

**The setup page** writes agent settings, so it is guarded: a random one-time token on disk
(`setup.json`, mode 0600, 24 h, deleted after one answer), constant-time compare, only the offered
question ids and option values accepted, the dashboard's loopback-Host and cross-origin checks.
Never opened under `CI`, without a display, or with `GASCOIGNE_SERVER=off`; those get flags and a
one-line notice. Rejected: a terminal-only prompt (blocks scripts, easy to miss in an agent's
output), an unauthenticated page.

**Where answers live.** `prefs.json` in the state dir, not `config.yaml`: a new config key needs a
change in `internal/config` (core, needs approval), and these are answers to questions, not tuning.
Env vars override where useful (`GASCOIGNE_GUIDE`, `GASCOIGNE_AUTOSYNC`).

## Permissions

**Levels `none | local | all`.** `local` = retrieve, show, stats, compress (read-only, local).
`fetch` reaches the network, so it is its own step. Default `none`: gascoigne does not loosen an
agent's approvals unasked. Rules added are recorded per settings file and kind (`denies.json`,
`allows.json`); `unwrap` removes exactly those and never a rule the user wrote.

## The usage guide

**Promise only what the agent has.** Rewrite advice only where commands are rewritten (not codex),
MCP tool names only where the MCP server is registered (not pi). Codex and pi share the project
`AGENTS.md` block; the last install wins (noted, accepted).

**Levels `off | short | full`**, unknown value = full: a typo must not silently remove the
explanation of the hint format. Brevity ("keep your own text brief") lives in the guide, delivered
at session start.

**Skills are user-invoked** (`disable-model-invocation`): their descriptions must not be sent with
every request; the project counts instruction tokens.

## Tried and dropped (reasons beyond the numbers)

- **Terse note in the proxy's system prompt** (`shape_output`). Measured about −16% output, but the
  guide's session-start line does the same, and editing the system prompt in the proxy touches what
  the API and the login terms treat as the client's own. Session-start context is the sanctioned
  channel. Removed.
- **Lowering thinking effort after tool results.** A changed thinking budget invalidates the cached
  message prefix, so alternating it per turn would cost more than it saves. Not built, not measured.
- **"Write a script, print only the result" guide line.** No effect on warm-cache runs (agents
  already answer such tasks with one shell command). Retry on harder tasks.

## Features kept experimental

- **`continuity`** (resume snapshot): built from the transcript at SessionStart after compaction or
  resume, so it needs no new hook registration; reads the agent's own calls, never tool output.
  Unmeasured (needs a compaction-forcing A/B).
- **`learn`**: only habits the ledger can show (reruns of shortened output, large unshortened
  output). Failed-command repeats need exit codes, which events do not record.

## Dashboard and costs

- **Model families by vendor prefix** (claude, gpt, gemini), not tokenizer: legacy and current
  Claude tokenizers differ but users think in vendors.
- **Cost figures are labeled estimates from the providers' official list prices**, with the check
  date and links (`cost.PricesChecked`, `PriceSources`); discounts and plans are not applied.
- **Tables sort by an explicit accessor**, not the displayed text (`12.3k` sorts as text wrongly).
  Sort and filter are not in the URL hash: it holds `#run-ID` links.
- **Savings are estimates, never billing**; every report that shows dollars says so.

## Process

- Diagrams are Mermaid in the Markdown (README, `docs/`, `.harness/`): renders on GitHub, diffs as
  text, nothing to regenerate. Rejected: ASCII art (breaks on edit, hard to read) and image files.

- Tasks live in `.harness/tasks/`; a finished task is deleted in the commit that does it.
- A build artifact (`cmd/gascoigne/gascoigne`) was committed by mistake and is now ignored; the
  history was not rewritten (unpushed commits, rewrite declined).
