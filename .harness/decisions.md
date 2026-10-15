# Decisions

Why things are the way they are, for the next person (or agent) who wonders. One entry per
decision: what, why, what was rejected. Measured results that closed a question are in
`backlog.md`; behavior is in `architecture.md`.

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

## Release notice

**On by default, minimal, and visible.** The website's `latest.json` (id and page) is read once a day
by the background server; the next session opens the page once. Chosen over asking first because a
notice nobody turned on is never seen. The cost is one request that shows the host the user's IP, so:
plain GET (no query, cookie, version or id), a 4 KB cap, a page that may not leave the site, hooks
never touch the network, one switch in setup and one env var, and the README says so. Rejected:
reading GitHub's releases API (rate limits, more data) and checking in every hook (latency, a request
per session).

**The website owns the release id.** `latest.json` changes when the page for a release is ready, not
when a tag is pushed, so a user is never sent to a page that does not exist yet.

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

Turning one on is a question in `gascoigne setup` (`setup -experimental`, or the page's last section),
stored in `prefs.json`, not an edit of `config.yaml`. It never counts as an open question: it is the
user's choice, so it must not bring the page back.

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

## MCP review

**Lazy tools, not hidden tools.** An unused tool is not removed from the agent: `mcp-proxy` stops listing
it and says in `search_tools`'s description that it exists, so the agent asks for it by name when the task
needs it. Rejected: hiding for good (an agent that is never told cannot ask) and a `list_changed`
notification when the agent asks (a changed tool set busts the prompt cache); the lazy set is read at
start and never changes inside a session.

**The choice needs no setup token.** It is saved from the review page with the dashboard's own guards
(loopback Host, same-origin writes) and no one-time token, unlike the setup page. The setup page changes
other programs' settings; this changes gascoigne's own `prefs.json` and is reversible in one click.
The endpoint accepts only a proxied server and tool names the review already shows for it.

**Server ids are name plus a command hash.** A server's reported name collides when one binary is
configured twice (two databases, two roots), so stats and choices are keyed by the name and a short hash of
the command line. Only the hash is stored: arguments can hold keys. The agent's config key is not known to
`mcp-proxy`, and passing it would change what `install` writes for every agent; it is found the other way
round instead: the config entry that runs `gascoigne mcp-proxy -- <command>` holds the command, so hashing it
gives the same id (`proxiedConfigs`).

**Remote MCP servers go through the stdio proxy, not a server route.** `mcp-proxy -url` connects downstream
with the SDK's HTTP/SSE client transports; the agent still starts it as a stdio command. Rejected: a
route on the background server with the upstream in a request header (a shared warm process, but a new
network surface to guard against loops and SSRF, plus a session cache). The cost is one process per agent
session, as for stdio servers. The id hashes the url (it can hold keys); header values live only in the agent's own config.

**OAuth is handled inside the proxy, through a login tool.** The agent's own OAuth tokens (kept by the agent, in
its keychain or files) are not readable by gascoigne, so a proxied OAuth server needs its own login, once. The
proxy has no terminal and an agent waits for it at start, so it neither blocks on a browser nor exits: it
offers `gascoigne_login` (instructions tell the agent to call it), and the server's tools appear when the
login is done. This costs one tools/list_changed in that session, the case where the alternative is no tools at
all. Rejected: probing servers at install and leaving OAuth ones direct (a network call per entry, and the user
runs a command anyway), and prompting on stdin (the proxy's stdin is the protocol). Tokens are plain JSON in the
state dir (0600), like the agents' own credential files.

**Switching a server off is shown, not done.** The review page says where the agent's config starts a server
and gives the command that removes it (`claude mcp remove <key> --scope user|local|project`; for Cursor, delete
the entry). Disabling would write another program's settings, which gascoigne does only after a question in
the setup page, and Claude's own file (`~/.claude.json`) is rewritten by a running Claude Code, so a write there
can lose data. The one-click action stays what is gascoigne's own: loading tools on request.

## Process

- Diagrams are Mermaid in the Markdown (README, `docs/`, `.harness/`): renders on GitHub, diffs as
  text, nothing to regenerate. Rejected: ASCII art (breaks on edit, hard to read) and image files.

- Tasks live in `.harness/tasks/`; a finished task is deleted in the commit that does it.
- A build artifact (`cmd/gascoigne/gascoigne`) was committed by mistake and is now ignored; the
  history was not rewritten (unpushed commits, rewrite declined).

## Layout

**Public surface is `component/ payload/ tokens/`; libs live in `internal/` (2026-10-15).** The OTel Collector
keeps a small public API and hides the rest. The top level had ~25 dirs (libs beside contracts and components)
and `optimizer/` nested one dir per family. Now `detect compress mcptool cost prose mcpauth version lazyre` are
under `internal/`, and optimizers are flat (`optimizer/<name>optimizer`; the family is the factory's
`WithFamily`, not a directory). Cost: components import `internal/`, so they can no longer become separate Go
modules; the old rule (components import only public packages) is gone. Kept: components never import the engine
(`internal/{config,router,service,registry}`), so no component special-cases core. Rejected: nesting libs
under `pkg/` (fewer dirs but still all public) and keeping libs public (no real consumer outside this module).
