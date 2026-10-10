# Ledger, retrieval and gain

Every run through gascoigne is recorded in a local sqlite ledger:
`$XDG_DATA_HOME/gascoigne/ledger.db` (override with `extensions.ledger.path` or
`GASCOIGNE_LEDGER`). It holds:

| Table | Holds | Kept |
|---|---|---|
| `raw` | originals of optimized payloads, by content-derived id | `raw_ttl` (24h by default) |
| `events` | every run: time, session, agent, model, command, source key, route, tokens in/out, outcome, the component that stopped it and why | forever |
| `steps` | per event, each optimizer that changed tokens: family, dialect, tokens before/after | forever |
| `failures` | per source key: how often it optimized badly, when, why | until `forget` |

The driver is pure Go (no cgo), in WAL mode, so several gascoigne processes (an agent's hooks,
the MCP server, the dashboard) share one ledger safely.

## The full-payload guarantee

This lives in the pipeline core, not in optimizers:

1. Before optimizers run, the payload is snapshotted.
2. If the output differs, the original goes to the ledger **first**, then the output gets its
   hint: `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. If saving the original fails, the original is returned instead.
4. If an optimizer errors, the original is returned and the failure recorded.
5. Output that was not changed is byte-exact and carries no hint.

## Getting originals back

| From | How |
|---|---|
| shell | `gascoigne raw <id>` |
| MCP server | `retrieve` with the id |
| MCP proxy | `gascoigne_retrieve` with the id |
| dashboard | open a run → *Reveal the original* |

Ids are derived from content, so the same original always has the same id.

## Sources that optimized badly

Each payload has a **source key**, the most specific of `file:<path>`, `url:<url>`,
`cmd:<command line>`, `tool:<name>`.

A source is marked as failing when:

| Signal | Reason recorded |
|---|---|
| an optimizer returned an error | `error` |
| an agent fetched the original (`gascoigne raw`, `retrieve`) | `raw_requested` |
| you said so: `gascoigne fail <key or raw id>` | `manual` (or `-reason`) |

The `history` filter then passes that source through untouched once it reached
`max_failures` (1 by default). Fetching an original is the signal: the optimized output was
not enough for that command, file or URL, so next time the agent gets it whole.

```sh
gascoigne gain -failures          # list failing sources
gascoigne forget 'cmd:go test ./...'
```

Viewing an original in the dashboard does **not** mark the source: a person debugging is not
an agent missing information.

## `gascoigne gain`

```sh
gascoigne gain
```

The overview shows:

- a totals line: tokens saved, saved %, runs by outcome;
- the last 7 days (quiet days included);
- savings by agent, by model (once recorded), by command (`go test`, `git log`, `fetch go.dev`),
  by optimizer (with its family) and by dialect;
- outcomes by share of input tokens: `optimized`, `unchanged`, `bypassed`, `error`;
- **skipped / failed / unchanged** runs, grouped by the component that stopped them and the
  command, each with its latest reason;
- how many sources are currently failing.

Colors follow saved %: green ≥ 60, yellow ≥ 30, red below. `-color never` or `NO_COLOR` turns
them off.

### Groupings and filters

```sh
gascoigne gain -by command
gascoigne gain -by dialect -since 7d
gascoigne gain -by day -since 4w
gascoigne gain -agent codex -model gpt-x -by optimizer
gascoigne gain -session claude-20261010-111234
gascoigne gain -json
```

`-by`: `day`, `week`, `month`, `agent`, `model`, `session`, `command`, `optimizer`, `family`,
`dialect`, `outcome`, `reason`, `route`. Filters `-session`, `-agent`, `-model`, `-since`
combine with any view.

Inside a wrapped agent `GASCOIGNE_SESSION` is set, so `gain` shows that session only; add
`-all` to see everything.

### Why a run was not optimized

Every run that was not optimized records a factual reason:

| Outcome | Reason looks like |
|---|---|
| bypassed by `bypass` | `raw requested: attr raw=true`, `env GASCOIGNE_RAW=1` |
| bypassed by `minsize` | `45 tokens < min_tokens 200` |
| bypassed by `history` | `cmd:go test ./... failed 1× (max_failures 1), last: raw_requested at …; reset: gascoigne forget cmd:go test ./...` |
| error | the first line of the optimizer's error, or `save original: …` |
| unchanged | the optimizer's own note (`cat: 590 tokens < clean_min_tokens 800`), or `no optimizer changed it` with the detected types |

`gascoigne gain -by reason` lists only these groups. The detected types in "unchanged" reasons
show which content has no engine yet.

## Dashboard

```sh
gascoigne dashboard                       # http://127.0.0.1:7878
gascoigne dashboard -addr 127.0.0.1:9000
```

One self-contained page served from the binary, styled after a gothic hunter's ledger:

| Panel | Shows |
|---|---|
| Totals | tokens saved, saved %, runs, optimized, unchanged or bypassed, errors |
| Nights of the Hunt | tokens saved per day |
| Outcomes | runs and share of input tokens per outcome |
| The Hunt — live | every run as it lands, newest first: agent, command, content type, tokens, saved, outcome |
| Prey / Weapons | savings by command; by optimizer and by content type |
| Why the Hunt Was Abandoned | runs not optimized, by stopping component and command, with the latest reason |
| Cursed Sources | failing sources, each with a *Cleanse* button (`forget`) |

Filters at the top narrow everything to one session, one agent, or a period. Clicking a run
opens its details: source key, pipeline and route, the optimizer chain, each step's
tokens before and after with its dialect, the stop reason, timing, and the original.

The live feed uses server-sent events and polls the ledger every second, since other
gascoigne processes write to it.

### API

The page is built on a small JSON API you can also script against:

| Endpoint | Returns |
|---|---|
| `GET /api/overview?session=&agent=&model=&since=` | the same data as `gain -json` |
| `GET /api/sessions` | sessions with their savings |
| `GET /api/events?after=<id>&session=&agent=&model=&since=` | up to 200 runs newer than `after`, newest first, with steps |
| `GET /api/stream?…` | server-sent events: the latest 100 runs, then each new batch |
| `GET /api/raw/<id>` | an original as text (does not mark the source) |
| `GET /api/failures` | failing sources |
| `POST /api/forget?key=<key>` | clear a source's failure history |

### Safety

The dashboard listens on `127.0.0.1` by default and shows originals, which may contain
secrets from command output. It answers only requests addressed to `localhost` or a loopback
IP (blocking DNS rebinding) and refuses cross-origin writes. Keep `-addr` on loopback.
