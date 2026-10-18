# Hooks and tool-result coverage

## Hooks

- Hooks call `gascoigne hook <agent>`, which never fails the tool call: bad input → no answer →
  call unchanged.
- **After-the-fact replacement** (`posthook.go`), for results the command rewrite cannot reach:
  - claude PostToolUse `updatedToolOutput` in the tool's shape (Bash stdout, Grep content mode, `mcp__*`)
  - codex PostToolUse `decision: block` + `reason` (`^Bash$|^mcp__.*`)
  - gemini AfterTool `decision: deny` + `reason` (shell, `mcp_*`)
  - skipped: hinted output (`hintRe`) and gascoigne's own MCP tools. Read is never replaced
    (Claude numbers its lines).
- **Permission rules** (`rules.go`): `install claude` adds deny rules like wrap (Grep, Glob,
  WebFetch) and, by the saved level (`install -permissions none|local|all`, `prefs.json`), allow
  rules for gascoigne's own tools: `local` = retrieve, show, stats, compress (read-only);
  `all` adds fetch (reaches the network); `none` (default) adds nothing. Added rules are recorded
  per settings file in `$XDG_STATE_HOME/gascoigne/denies.json` / `allows.json`; unwrap removes only
  recorded ones, a changed level drops the old ones first, a rule the user already had is never
  taken.
- **Install manifest and sync** (`installs.go`): `install` records each agent, scope, project dir,
  how the config calls gascoigne, `-proxy` and a `registration` number in `installs.json` (state
  dir); `unwrap` drops them. `registrationVersion` is bumped whenever what install writes (hooks,
  matchers, guide, rules) changes. On SessionStart/UserPromptSubmit hooks, when `prefs.autosync`
  is `on` (`install -autosync on`) and `$GASCOIGNE_AUTOSYNC` is not `off`, user-scope installs with
  an older number are written again (`agent.sync`, else `add`; claude: settings only, never
  `~/.claude.json` which a running Claude Code rewrites), under `installs.lock`, never failing the
  hook; SessionStart tells the agent to restart. Project installs are never touched by hooks:
  `gascoigne upgrade` does this project's and the user's now. Not decided = nothing is synced.
- **Release notice** (`release.go`): the session and prompt hooks and `dashboard` (`checkReleaseQuick`, 2s cap)
  read `<site>latest.json` (`{"id":"0.3.0","released":"…","page":"releases/0.3.0/"}`) every 15 min
  (`checkRelease`; a failure is retried in 15 min, the last good answer stays) and saves it as
  `release.json` in the state dir. A claude SessionStart or UserPromptSubmit hook (`releaseNudge`: not compact or clear) opens the page once per release when `version.Newer(id, version.Number)`
  (`prefs.seen_release`, saved only once the page opened, so a session without a desktop tries
  again next time; the browser is started detached, as the hook exits at once) and adds a note.
  The server never fetches. The id must be a plain
  `X.Y.Z`; the page must stay on the site's host and path; the body is cut at 4 KB; the request has
  no query, cookie or version. Off: `GASCOIGNE_RELEASE_CHECK=off`, `prefs.release_check`, or no server.
  The setup question is optional (`on` until answered off). `doctor` and `upgrade` show a newer
  release. The website side is the `gascoigne-webpage` repo: `latest.json` plus `releases/<id>/`.
- **Weekly MCP review** (`mcpnudge.go`): `mcpNudge` runs after `setupNudge` and `releaseNudge` in a claude
  SessionStart (startup or resume). It returns nothing when either of them opened a page (one page per
  start; the week is not used up), on compact or clear, with `GASCOIGNE_MCP_REVIEW=off`,
  `prefs.mcp_review: off` or `GASCOIGNE_SERVER=off`, or less than 7 days after `prefs.last_mcp_review`
  (read before any request). Then it asks the running server (`GET /api/mcp`, 1 s timeout, never starts
  it) and, when `reviewWorth` finds at least `mcpNudgeTokens` (1500) per request in unused proxied
  servers and idle tools not yet chosen to load on request, records the week, opens `/mcp` (not under
  `CI`) and adds one note. Direct servers and servers seen under a week never count. The setup question
  `mcp_review` is optional and on by default, like `release`.
- **Legacy installs** (`installs.go` `legacyInstalls`): installs made before `installs.json` existed
  (the first release) are found by their config: the user-level file, and this project's, holding
  `hook <agent>` while nothing records it. `readInstalls` returns them as registration 0, so the
  update question, `upgrade` and `doctor` treat them as stale; recording an install (or `upgrade`)
  makes them regular entries.
- **Decisions** (`setup.go`, `setup.html`): changes beyond gascoigne's own files wait for the
  user. Open questions = unset `allow` / `autosync` in `prefs.json`. `install` (end) and a claude
  SessionStart hook (source startup or resume, not compact or clear; at most every ten minutes,
  `prefs.asked_at`; again at each new session until every question is answered; recorded installs
  only) open `http://<server>/setup?token=…` with `openBrowser` (not under `CI`, not without a
  display; `GASCOIGNE_SERVER=off` falls back to the one-line notice). `GET /api/setup` and
  `POST /api/setup {token, answers}` need the token from `setup.json` (random, 24h, mode 0600,
  deleted after one answer; constant-time compare); only the offered ids and values are accepted;
  the usual loopback-Host and cross-origin checks apply. An answer saves the prefs and writes
  user-scope installs again; project installs are listed ("run gascoigne install there").
  `gascoigne setup [-y] [-all] [-t]` opens the page, or asks the same in the terminal. The page
  (`setup.html`) first shows what was found (`buildSetupView`: each install connected or older, agents
  on PATH not connected or declined, the version), then the questions in sections by `decision.Group`
  (permissions, updates, agents), each option with a sentence saying what it does (`option.Detail`);
  experimental features are optional questions (`decision.Optional`, always answered, never nag), folded
  on the page, each with an example, how it works and its impact (`decision.Example/How/Impact`,
  also printed by the terminal form): the
  choice is `prefs.experimental`, which `experimentalOn` reads next to `$GASCOIGNE_EXPERIMENTAL` and
  the config's list; `setup -experimental` asks only those, and a test keeps `experiments` in step with
  the commands marked experimental in `help.go`;
  a further optional question, `prose_model` (`proseModelDecision`, choices in `proseChoices`), picks the
  prose model: `prefs.prose_model` is laid over `optimizers.prose.model` by `configData`, so
  `configProseModel`, `prose install`, `prose status` and the optimizer all follow it; when prose is on,
  save verifies the chosen model's files (`needsProseModel`) and downloads them if missing or corrupt;
  a save bar says how many answers will be saved, and the result lists what was done.
  New agents: an agent on PATH with no recorded install and not declined gets a question
  (`integrate:<agent>`, default yes). It is only signaled: a SessionStart note and the page (until answered, like any question); yes runs the user-level install and records it, no is remembered
  (`prefs.declined`; `setup -all` can reverse it). Nothing is installed without the answer.
- **Doctor, upgrade and skills** (`doctor.go`, `upgrade.go`, `claudeskills.go`): `doctor` only
  reports: binary on PATH, server, guide level, open questions and, per recorded install, whether
  the agent's config (`configOf`) still holds `hook <agent>` and the registration is current. Each
  problem carries a `fix`, which `gascoigne upgrade [-n]` runs: it is the one place for what a new
  version needs (stale or lost hooks, rules and commands of user installs and this project's; other
  projects are named; `config.yaml` moved onto the template via `writeUpgrade`, `.bak` kept; open
  choices handed to `offerSetup`). `install claude` also writes the skills `gascoigne-stats` and
  `gascoigne-doctor` (`disable-model-invocation`, so their descriptions are not sent with requests;
  marked with `skillMarker`, so `unwrap` removes only ours).
- **Identity**: rewritten commands carry `-agent <name>` and `-session <hook session_id>` when
  `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` are unset (installed hooks). `wrap` sets the env, so its
  commands stay short (`execPrefix` in `hook.go`).

## Coverage (which tool results reach the optimizer)

| Tool | claude | codex | gemini | opencode / pi / cursor |
|---|---|---|---|---|
| shell | rewrite (PreToolUse) | PostToolUse replaces result | rewrite (BeforeTool), AfterTool replaces what the rewrite missed | rewrite before the call |
| Grep, Glob | denied → `rg`/`find` through the shell | shell only | shell only | shell only |
| WebFetch | denied → gascoigne `fetch` (MCP) | no hook | no hook | no hook |
| Read | not replaced: the hooks accept a replaced result of the tool's shape, but Claude numbers the lines itself, so only line-preserving changes are safe, and they save under 1% (see backlog); the LLM proxy shortens it when on | no hook | no hook | no hook |
| MCP tools | PostToolUse `mcp__*`, plus `mcp-proxy` | PostToolUse `mcp__*` | AfterTool `mcp_*` | `mcp-proxy` only |
| Subagents | hooks run there too: gascoigne's own PreToolUse and PostToolUse show in subagent transcripts (13 files, 2026-10-12) | unverified | unverified | unverified |

Cursor has the preToolUse hook only: no result replacement is known. A status line can show the
savings: `gascoigne gain -oneline -since 1d` (`docs/agents.md`).

## Resume snapshot (`continuity.go`, experimental: `continuity`)

Claude SessionStart with `source` compact or resume adds a snapshot of the transcript to the
guide's context: last request, open TodoWrite items, files edited (newest first, 15), recent
failed Bash commands (3, `NAME_KEY=` / bearer values cut). Only the agent's own calls are read,
never tool output; subagent lines are skipped; capped near 2 KB and deterministic. It also names the
outputs gascoigne shortened in this session (up to 6, one per command, latest raw id, at least 200 tokens
saved; `ledger.RecentRuns` by the hook's session id, the default ledger path, command and id only) so that the
agent reads them with `gascoigne raw <id> -grep` instead of running the command again. Needs no new
registration (SessionStart is already installed). Gemini's `PreCompress` has no equivalent yet.

## Command rewriting rules (`hook.go`)

- Shared by every rewrite agent (claude, cursor, gemini, opencode, pi); codex and gemini's AfterTool use only the
  detect side (`detect.FromCmd` on the command that ran).
- **Wrapped**: every command of a `&&`/`;` list, except builtins, quiet, interactive and REPL
  commands (`passthrough`; unknown programs are detected by content).
- **Kept pipes**: `| head` / `| tail` stages at the end of any command of a list (they then cut
  optimized text). Before `head`, exec gets `-attr hint=top`: the stdout exporter prints the hint first so
  the cut keeps it. Other pipes stay as they are, since other filters would search the optimized text; one
  such command leaves the whole line alone. `| & < >` inside quotes are literal (`grep 'a\|b'`); a `$(` or
  backtick inside double quotes still runs, and a quote that never closes leaves the line alone
  (`hasShellOperator`). Replayed on 306 commands of one real session this wraps 93 instead of 56.
- **Left as they are**:
  - lines with loops, conditionals or `{ }` groups
  - redirects (except `2>&1`), subshells, `||`, background runs
  - commands that run gascoigne itself
  - commands that run until stopped (`-f`/`--follow`, `--watch`, `dev`, `serve`, `start`, `up`,
    plain `vite`), since exec shows output only when the command exits
- **Watch mode exception**: build tools in watch mode (`watched`) run as `exec -watch`, which
  processes each cycle when `watch.End` sees its result line; background runs included.
