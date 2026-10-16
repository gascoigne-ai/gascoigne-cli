# Dashboard and review pages

## Context report (`context.go`, `context.html`)

`GET /context` over `GET /api/context?since=24h|7d|…` (default 14d, 1 hour to 90 days) = `loadContext(~/.claude/projects, period)`:
the newest transcripts (at most 300) written within the period, read once each, each cut to the turns inside it. Not the ledger: only transcripts carry a
session id and per-turn usage (the LLM proxy has no session).
- `readSession`: assistant lines of the main conversation (sidechains and synthetic messages skipped; a message
  streamed twice counts once, the last usage wins); context = input + cache read + cache write; a drop below
  `compactDrop` (60%) of the previous turn, from `minContext` (60k) up, is a compaction or `/clear`.
  Model names are normalized (`anthropic/claude-sonnet-5.5` → `claude-sonnet-5-5`) before `cost.Lookup`.
- Cost at list prices, 5-minute cache writes. Buckets split the cost by the context of the request.
- `simulate(turns, limit, floor, model)` re-prices a session as if compacted when its context passed `limit`
  (150k, 250k, 400k; a conversation restarts at `floor`, the smallest context of the whole session even before the
  period, and a limit it could not shrink below is skipped): reads for what was cached, writes for what is new, one full-context request plus a
  12k summary per compaction, growth as it was; a real reset is followed, not compacted twice. The same model
  prices the baseline, so thresholds compare with each other (within 5% of the actual cost on a real
  month). More compactions are not always cheaper.
- Cache rewrites (`rewrites`): a turn whose cache read fell short of the previous request's context by over 5%
  (and 2k tokens) rewrote the difference, priced at write minus read. Cause from the transcript's timestamps:
  compaction, a pause over an hour, a pause of 5 to 60 minutes; within the cache lifetime: another model
  than the request before (the cache is per model), a context that shrank without a compaction (older content
  removed or rewritten), only a fixed start read (the rest rewritten), a change recorded between the requests
  (tool set, instruction files, skill, agent or MCP listing: evidence, not proof), else unexplained.
  Request bodies (the proxy) would name the system prompt or tool-set divergence; transcripts cannot.
- Injected text (`injectedText`): from `attachment` lines. A hook's `content` (SessionStart, named by the command it ran:
  the last two parts of its first path), a prompt or tool hook's `additional context` (the event only: the
  transcript does not name the hook), instruction files, the skill listing, agent listing and MCP instructions.
  Priced as written once and read by every later request of the session (`before` = requests already made).
  The settings audit: a plugin hook's cost per month is in the table.
- Subagents (`<session>/subagents/agent-*.jsonl`, every line a sidechain) are not sessions: their requests are priced
  apart (`addSubagent`, `subagent_usd`, a KPI on the page) because their context is not the conversation's.
- Everything local and read-only; the page says compacting saves tokens, not detail.

## MCP review (`mcpreview.go`, `mcp.html`)

`GET /mcp` (one embedded page, same design tokens as `setup.html`, DOM built without `innerHTML`,
sort by accessor) over `GET /api/mcp` = `buildReview(ledger.ToolStats, ledger.MCPCommands, now)`:
- Per server: definition tokens (last listing), tokens of tools not called in 14 days, calls in the
  last 7 days and the 7 before, last use, and a verdict. `learning` below `observeDays` (7) of
  observation, `not seen lately` when a proxied server was last listed `staleDays` (7) or more days ago (probably
  removed: never unused, never in the nudge), `unused` with no call in `unusedDays` (14), `rarely used` for at most `rarelyCalls`
  calls a week at `rarelyTokens` or more per request, else `used`. The constants are named in the
  file; days are UTC.
- Calls to servers that stay direct come from the hook's runs named `mcp__server__tool`
  (`MCPCommands`); a direct server is dropped only when every tool the hook saw it call is a tool one
  proxied server lists (`coveredByProxy`: the proxy already counted those calls); gascoigne's own
  server and the proxy's lazy tools (`call_tool`, `search_tools`) are left out; Gemini's `mcp_server_tool` naming cannot be split and is skipped.
- A direct server that was never called is invisible (no list, no calls): the page says so.
- `gascoigne mcp-review` prints the page address and opens it (`canBrowse`: display, no `CI`, server
  up); otherwise, or with `-text`/`-json`, it prints from the ledger.
- Where a server is configured (`mcpconfig.go`): `proxiedConfigs` reads `~/.claude.json` (user and local servers),
  project `.mcp.json` and Cursor's `mcp.json` (project dirs from `installs.json` and the working directory),
  finds the entries that run `mcp-proxy -- <command>` and indexes them by `mcpproxyreceiver.CommandHash(command)`,
  which is the suffix of the server's id. The page shows each entry and the removal command; nothing is written.
- The one action: `POST /api/mcp/lazy {id, tools}` (`setLazy`) saves `prefs.lazy_tools[id]`, which
  `cmdMCPProxy` passes to the proxy (`lazyChoices`; `GASCOIGNE_MCP_LAZY=off` ignores it). It accepts only a
  server with sizes (started through `mcp-proxy`) and tool names the review shows for it; an empty list
  clears. The page offers it for tools not called in 14 days and says it applies from the next session
  (the recorded lazy flags lag the choice until then). Reasons and rejected options: `decisions.md`.

## Costs page (`costs.html`, `GET /costs`, `GET /api/costs`)

Own page, shared `pages.css`/`pages.js` and nav, Period filter only. Two parts:
- Estimated from tokens saved (`mcptool.BuildCosts`, same `overview.cost` data): families (claude/
  gpt/gemini/other from `cost.Model.Family`, summed in the page) and models. "Saved" is in the
  tokens.Estimate units the dashboard uses, so a family is the sum of its models; "In model tokens"
  applies the model's tokenizer factor. Models come from the run (proxy request, Claude transcript);
  none recorded → the agent's default, marked assumed. Dollars: first read and in context.
- Measured by the proxy (`/api/usage`: requests, context, cache per day and model); empty when
  the agent's login bypasses the proxy.

## Dashboard (`dashboard.go`)

One embedded `dashboard.html`, no build step, serving the ledger live.
- Tables (`lazyTable`): click a header to sort (numbers by value through a column's `sort`
  accessor; first click largest or A-Z, second reverses, third restores the server's order) and a
  filter box on every table; both survive live refreshes. The recent-runs feed does the same on
  the runs loaded so far (`feedCols`; while sorting or filtering the body is rebuilt from all
  loaded runs, and older pages keep loading so a rare match can appear). Not kept in the URL
  (its hash holds `#run-ID` links). `dashboard_js_test.go` runs the helpers under node.
- Layout: version in the header, the logo moon's phase = savings rate; tables in equal-height
  boxes, rows rendered as they scroll; deleting data (reset, clear originals) asks twice.
- Content: totals, days, outcomes, commands, optimizers, dialects, skip/fail/unchanged reasons,
  failing sources (forget button), and a feed of runs over server-sent events (`/api/stream`,
  polls the ledger since other processes write it; `ledger.Recent`).
- Filters (`picker`): the Session, Agent and Period selects are drawn as a button and list; the
  select stays the value and fires `change`. An option may carry `data-label`, `data-meta` (figure
  on the right), `data-sub` (detail line) and `data-share` (bar): sessions show saved tokens, runs,
  last activity; agents show saved tokens, runs, share saved. Money lives on `/costs` only.
- Trend charts: `overview.days` is per day up to 32 days, per ISO-like week up to ~190 days,
  then per month (`mcptool.chartBuckets`); past 12 bars the page hides amounts (tooltips keep
  them) and shows about 10 labels (`thin`). `/api/impact` stays per day and thins the same way.
- Session list (`/api/sessions`): follows the Period and Agent filters (not Session), latest
  session first (`max(ts)`, not savings); a selected session without runs in the period stays listed.
- Run details compare original and optimized side by side (`/api/raw/{id}`, `/api/out/{id}`; the
  optimized text is `Event.Output`, kept gzip in `raw.output`), with `#run-ID` deep links. A run
  opens its steps and original; viewing an original there does not mark the source as failing.
- Security: loopback Host names only (DNS rebinding); cross-origin writes refused; the server refuses a non-loopback `-addr` (no login); `/api/optimize` rejects a config whose ledger is not the server's before opening it; state files 0600, dirs 0700; `nosniff` + `X-Frame-Options: DENY`; the proxy forwards bodies over 128 MB unrewritten.
