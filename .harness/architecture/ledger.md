# Ledger (gorm + sqlite, `extension/ledger`)

## Storage

- Pure-Go driver (`gormlite`), WAL mode, busy timeout 5 s.
- Gorm models in `models.go`; `AutoMigrate` on Start adds new tables, columns and indexes.
  Renames, drops or type changes need an explicit migration step: ask first.

| Table | Holds |
|---|---|
| `events` | every trip: route, tokens, outcome, `session`, `command`, `reason`, bypassing `filter` |

**Rerequested**: an `optimized` event becomes `rerequested` when the agent then asks for the original: a full fetch (`MarkFailure` with `raw_requested` marks the source's latest optimized run) or a raw rerun (`Record` of a bypass with a `raw requested` reason marks the earlier optimized run of that source and session). `migrateRerequested` applies this once to old data (`user_version` 2). Stats count a rerequested run as the agent read it: the optimized result plus the original (out = out + raw), so savings net out the give-back; `Ledger.Impact` / `GET /api/impact` split gross savings and give-back per day for the dashboard chart. The dashboard's Raw filter is the other case: a raw request with no earlier run of that source.
| `steps` | per event, each optimizer that changed tokens: optimizer, family, tokens before/after |
| `raw` | originals, gzip-compressed (plain JSON rows from before still read); TTL-pruned. `gascoigne flush [-older]` / `POST /api/flush` delete and VACUUM |
| `failures` | per-key failure count |
| `tool_stats` | per day, agent, MCP server, tool: definition tokens (last listing that day) and calls. Written by `mcp-proxy` through the optional `mcptool.ToolStatRecorder` (not a `component/` contract); pruned at `stats_ttl` (1 y). Direct servers' calls are the hook's events whose command is the tool name |
| `usage` | per proxied API request: agent, model, input, cache read/write and output tokens as the provider billed them |

Retention (pruned once, when the ledger starts): `raw` at `raw_ttl` (30 d), `events` with their `steps`, `usage` and `failures` at `events_ttl` (1 y), `tool_stats` at `stats_ttl` (1 y); `0` keeps forever.

**Raw ids** are sha256 prefixes of the original's text, attrs aside: the same output from two
sessions is stored once. 8 hex digits, unless that prefix holds another text (then 12, 16, 64):
~7 fewer tokens in every hint, still content-addressed and unique.

**Source keys** over 200 bytes (whole command lines, heredocs) are stored as a 160-byte prefix +
`…#<hash>` (`keys.go` `storedKey`, applied by every method). Ledgers from before were converted
once (`PRAGMA user_version` 1) and vacuumed: 22 MB → 6 MB on a real one, mostly the key index.

## Identity and sessions

- `-session ID`, `-agent NAME`, `-model NAME` on any command (env `GASCOIGNE_SESSION`,
  `GASCOIGNE_AGENT`, `GASCOIGNE_MODEL`) tag events.
- MCP: agent defaults to the client's name (clientInfo); model only when given. Session: explicit
  `-session`, else the MCP transport session id, else one id per server run.
- Hooks record the model from the tail of `transcript_path` (`transcriptModel`, `-model` in the
  rewrite).
- `events.command` is normalized via `detect.Command`, else MCP tool + URL host.

## Lessons: `gascoigne learn` (`learn.go`, experimental)

Reads the newest runs (default 30 days, 5000) and finds two habits, each in at least two
sessions: `rerun` (the same key shortened with a hint three or more times in a session: the agent
runs the command again instead of `gascoigne raw <id> -grep`) and `big` (output of 1500+ tokens
that no optimizer shortened). Proposed lines are deterministic, at most 10, printed by default;
`-apply` writes a marked block (`<!-- gascoigne:learned:start/end -->`) to `CLAUDE.local.md` or
`-target`, `-remove` takes it out (the file goes if nothing else is in it). Proxy folds do not
count. Nothing is written otherwise.

## Reports: `gascoigne gain`

| Command | Shows |
|---|---|
| `gain` | overview: totals line; last 7 days trend (quiet days shown); by agent; by model (once recorded); by outcome (optimized/unchanged/bypassed/error, with tokens and share of input); by command (`go test`, `git log`, `fetch go.dev`); by optimizer (with family); skipped / failed / unchanged with reasons; count of failing sources |
| `gain -by day\|week\|month` | trend, oldest first, bars scaled to the best period |
| `gain -by agent\|model\|session\|command\|optimizer\|family\|outcome\|reason\|route` | one grouping |
| `gain -failures` | sources that bypass optimization after going bad |
| filters, any view | `-session`, `-agent`, `-model`, `-since 7d\|4w\|6mo\|24h` (combine freely); also `-all`, `-json` |

- The overview ends with `estimates` (`Overview.Estimates`, JSON `estimates`, MCP `stats`): what
  is estimated and how (tokens.Estimate error, cost factors and rereads, image pixels/750, tool
  sets counted once), so totals are not read as billing.
- Terminal output (`cmd/gascoigne/render.go`): human token counts, savings/share bars, colors by
  saved % (green ≥ 60, yellow ≥ 30, red). `-color auto|always|never`; auto honors `NO_COLOR` and
  TTY detection.
- MCP `stats {by?, session?, format?: json}` stays plain text (no ANSI or bars: they cost the
  agent tokens) and defaults to the overview of the calling session (`session: all` for every
  session).

## Cost (`internal/cost/`, `mcptool.BuildCosts`, docs/cost.md)

saved tokens × tokenizer factor × (first read + rereads), per agent and model:
- tokenizer factor: claude 1.58 (measured from transcripts), legacy 1.25, gemini 0.79, o200k 1
- first read: cache write or input price; then `rereads` (measured median 110 later requests in
  context) × cache read price; 0.8% misses at write price
- runs without a model use `cost.AgentModels` (marked assumed)
- prices are the providers' official list prices (`cost.PriceSources`, `cost.PricesChecked`);
  `Costs.Pricing` carries that basis into the overview JSON, `estimates` and the dashboard

## Skip reasons

Every run that is not optimized records a factual reason (`events.reason`), shown per component
and command in the overview's SKIPPED / FAILED / UNCHANGED sections and in JSON. `gain -by reason`
lists these groups alone.

| Source | Example reason |
|---|---|
| bypass | `raw requested: attr raw=true` or `env GASCOIGNE_RAW=1` |
| minsize | `45 tokens < min_tokens 200` |
| history | `<key> failed N× (max_failures M), last: <reason> at <time>; reset: gascoigne forget <key>` |
| optimizer error | first line of the error |
| ledger failure | `save original: …` |
| unchanged | optimizer note (e.g. `cat: 590 tokens < clean_min_tokens 800`) or `no optimizer changed it`, plus the detected types (which content lacks an optimizer) |

Filters and no-op optimizers explain themselves via the `skip_reason` attr (`payload.AttrSkip`).

## Failure history

Signals → `MarkFailure(key, reason)`: optimizer error (`error`), agent fetched the raw original
(`raw_requested`), `gascoigne fail` (`manual`). The `history` filter bypasses a key once
`count >= max_failures`; `gascoigne forget <key>` resets it.
