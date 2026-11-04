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
  (100k to 400k: 100k, 150k, 200k, 250k, 300k, 400k, `thresholds`; a conversation restarts at `floor`, the smallest context of the whole session even before the
  period, and a limit it could not shrink below is skipped): reads for what was cached, writes for what is new, one full-context request plus a
  12k summary per compaction, growth as it was; a real reset is followed, not compacted twice. The same model
  prices the baseline, so thresholds compare with each other (within 5% of the actual cost on a real
  month). More compactions are not always cheaper.
- Compaction economics (`compactionEconomics`, JSON `compactions`, a line in `context -text`): per compaction the cost (write of the new
  context plus a `summaryTokens` summary) and the saving (cache reads of the shorter context by every request until the next
  compaction); by the same simple model as `simulate`, priced sessions only. Measures what compaction paid, not what it forgot.
- Cache rewrites (`rewrites`): a turn whose cache read fell short of the previous request's context by over 5%
  (and 2k tokens) rewrote the difference, priced at write minus read. Cause from the transcript's timestamps:
  compaction, a pause over an hour, a pause of 5 to 60 minutes; within the cache lifetime: another model
  than the request before (the cache is per model), a context that shrank without a compaction (older content
  removed or rewritten), only a fixed start read (the rest rewritten), a change recorded between the requests
  (tool set, instruction files, skill, agent or MCP listing: evidence, not proof), else unexplained.
  Request bodies (the proxy) would name the system prompt or tool-set divergence; transcripts cannot.
- 1-hour cache advice (`cacheTTL`, JSON `cache_ttl`, a line in `context -text`; no page yet): the tokens rewritten after
  a pause of 5 to 60 minutes would have been read (saved: write minus read price) against every 5-minute write
  priced as an hour write (extra: 2x input minus the 5-minute write). `worth` from `ttlMinTurns` (200) requests when
  saved is at least `ttlMinGain` (1.15) times extra. Advice only; exact price per session, not the blended one.
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
- Per server: definition tokens (last listing), tokens of tools not called in `unusedDays` (7), calls in the
  last 7 days and the 7 before, last use, and a verdict. `learning` below `observeDays` (2: the first day counts) of
  observation unless the user asked "learn now" (`prefs.learned_mcp`, set by `POST /api/mcp/learn {id}`, no id = all
  still learning, or `mcp-review -learn`; same loopback-only, cross-origin-refused rules as the lazy choice),
  `not seen lately` when a proxied server was last listed `staleDays` (3) or more days ago (probably
  removed: never unused, never in the nudge), `unused` with no call in `unusedDays` (7, the `Calls7` window), `rarely used` for at most `rarelyCalls`
  calls a week at `rarelyTokens` or more per request, else `used`. The constants are named in the
  file; days are local days.
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
  clears. The page offers it for tools not called in 7 days and says it applies from the next session
  (the recorded lazy flags lag the choice until then). Reasons and rejected options: `decisions.md`.
  Manual rules: `POST /api/mcp/rules {id, rules:[{match, action}]}` (`setRules`) replaces one proxied server's
  `prefs.mcp_rules` (validated by `Rule.Validate`, at most `maxRules`, empty clears); the review reports them as
  `rules`, `mcp-proxy` reads them with `WithRules(savedRules())` (`GASCOIGNE_MCP_LAZY=off` ignores them), and the
  page's "Keep raw" tick is the exact-name `skip` rule.
- Order: servers and a server's tools come most called first (calls in 7 days, then 14 days or tokens, then
  name); the page opens sorted by calls and every heading, also of the tool table, sorts by accessor.
- Suggestions (`mcpsuggest.go`, `suggestMCP`, per server in `suggestions`): tools not called in 7 days with a
  definition of 100 tokens or more are suggested to load on request when together they beat the price of lazy
  mode (500 tokens; waived once the server has lazy tools); tools a rule names are the user's call. Servers
  learning, not seen lately or direct get none. Advice that lives in the agent's config, which gascoigne does not
  write, is shown only (`serverAdvice` table): switch an unused server off, `max_tools` from 20 listed tools,
  `trim_descriptions` from an average of 300 tokens.
- Auto-optimize: `POST /api/mcp/optimize {id?}` (`optimizeMCP`, same loopback and origin rules) decides from the
  ledger alone (it ignores anything else in the body), adds the suggested tools to `prefs.lazy_tools[id]`
  (never removes a choice, never touches rules) and returns the review plus `applied`; a second call applies
  nothing. No id = every server; an unknown or direct id is a 400. `mcp-review -optimize` does the same for all;
  `-text` lists the suggestions.

## Costs page (`costs.html`, `GET /costs`, `GET /api/costs`)

Own page, shared `pages.css`/`pages.js` and nav, Period filter only (same periods as `/context`, default 14d).
- Estimated from tokens saved (`mcptool.BuildCosts`, same `overview.cost` data), one model table.
  "Saved" is in the model's own tokens (tokenizer factor applied). Models come from the run (proxy
  request, Claude transcript); none recorded → the agent's default, marked assumed; the page states what
  share of the in-context dollars is assumed. Dollars: first read and in context, API-equivalent (not
  spend); the reread count is stated in the KPI note. Gemini's tokenizer factor is marked as not measured.
  Each model row says where its price comes from (built-in, static, json, api, script). Billing `subscription` replaces the tables
  with a note (no dollars); `mixed` keeps them and names the agents left out. A read-only Pricing panel (`GET /api/pricing`,
  `POST /api/pricing/refresh`) shows the billing mode, what auto found per agent, each source's last good run, next run and error, a
  "Run now" and a "Change pricing in setup" button (opens `/api/setup/link`). Settings are not edited here: see ledger.md, User pricing.
  Recorded eval runs (agent `eval`) are left out. The proxy's measured requests (`/api/usage`) moved to
  the Context page ("Requests through the proxy", shown when any exist).
- Charts (`G.hbars`, `G.bars`, `G.area` in `pages.js`): Costs draws the in-context dollars per model as bars (orange = assumed model); Context draws cost per local day (`days` in the report, quiet days included), a share-of-cost bar per session, and a detail row per session (click) with its facts and a context-over-time area chart marked at 200k. Optional notes are filtered before `replaceChildren` (a null prints as "null").
- On a flat plan (`contextReport.Billing` is `subscription`: `cost.BillingOf("claude")`, forced or detected) the Context page and
  `gascoigne context` draw requests and tokens, no dollars: cost columns, cost KPIs, the money chart, the what-if and the
  default sort by cost are replaced or dropped (`isFlat()`, `printContextFlat`).
- The Context page says it covers Claude Code only, its Cost is the main conversation (the total with
  subagents is a tile beside it), and cache-rewrite causes are "likely".

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
