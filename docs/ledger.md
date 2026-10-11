# Ledger, retrieval and gain

Every run through gascoigne is recorded in a local sqlite ledger:
`$XDG_DATA_HOME/gascoigne/ledger.db` (override with `extensions.ledger.path` or
`GASCOIGNE_LEDGER`). It holds:

| Table | Holds | Kept |
|---|---|---|
| `raw` | originals of optimized payloads, gzip-compressed, by content-derived id | `raw_ttl` (24h by default), or until `flush` |
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
| dashboard | open a run → *Show original output* |

Ids are derived from content, so the same original always has the same id.

Originals are stored gzip-compressed (command output shrinks about 9×; rows saved before
compression are still read). To drop them sooner than `raw_ttl`:

```sh
gascoigne flush               # every stored original
gascoigne flush -older 2h     # only originals saved more than 2 hours ago
```

`flush` vacuums the database, so the space goes back to the disk. Runs and savings stay; only
`gascoigne raw <id>` for the flushed ids stops working. The dashboard's *Clear originals* button
(it shows the count and size) does the same.

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

## Resetting stats

```sh
gascoigne reset -session <id>     # or -agent, -model, -since 24h; none: every run
gascoigne reset -all              # also stored originals and failure history
```

A reset deletes recorded runs and their steps. Stored originals and failure history stay
unless you pass `-all`, so `raw <id>` hints already shown to an agent keep working. The
dashboard's *Reset statistics* button does the same for its current filters.

## Dashboard

```sh
gascoigne dashboard               # http://127.0.0.1:7878
```

### The background server

The dashboard is served by one background process per user, like a build daemon:

- **Always there.** `exec`, `hook`, `mcp`, `mcp-proxy` and `wrap` start it when nothing answers
  on its address. `wrap` prints the address before the agent starts. Checking costs about
  0.3 ms per command.
- **One instance.** The listening port is the lock: a second server cannot start.
- **Follows upgrades.** The server checks its binary every 2 seconds and restarts itself once a
  rebuilt or upgraded one is in place. A command from a newer gascoigne elsewhere (another path)
  replaces the server too; an older one leaves it alone and optimizes by itself.
- **Keeps the pipeline warm.** `exec`, `run` and the codex hook run the command themselves and
  hand its output to the server, which keeps config, optimizers and the ledger open between
  runs (about 1 ms and 40% CPU less per command). The server only takes requests from the same
  build and ledger, and runs with `GASCOIGNE_RAW`, `GASCOIGNE_CAT_MODE` or `GASCOIGNE_LEDGER`
  set stay local.
- **Stays out of the way.** With no server, another build, or a server that fails, the command
  builds the pipeline itself, so a stopped or broken server never fails an agent. A server
  serving another ledger (for example an isolated `GASCOIGNE_LEDGER`) is left alone.
- **Exits when idle.** After 30 minutes without a request (health checks don't count, an open
  dashboard does) it shuts down; the next command, or an agent's next prompt through its
  UserPromptSubmit hook, starts it again. `gascoigne server run -idle 2h` or
  `GASCOIGNE_SERVER_IDLE=2h` changes it, `0` keeps it running.
- **Detached.** It runs in its own session and outlives the terminal or agent that started it.
  Its log is `~/.cache/gascoigne/server.log` (`$XDG_CACHE_HOME`).

```sh
gascoigne server status           # pid, uptime, ledger, build
gascoigne server stop             # or the Stop server button in the dashboard
GASCOIGNE_SERVER=off              # never start it
GASCOIGNE_SERVER_ADDR=127.0.0.1:9000
```

### The page

One self-contained page served from the binary, with no external requests. It follows the
system's light or dark theme; the *Auto / Light / Dark* switch in the header overrides it and is
remembered per browser.

| Panel | Shows |
|---|---|
| Summary | tokens saved, savings rate, runs, optimized, not optimized, errors |
| Daily savings | tokens saved per day |
| Outcomes | runs and share of input tokens per outcome |
| Recent runs | live feed, newest first: agent, command, content type, tokens, saved, outcome |
| By command / By optimizer | savings by command; by optimizer and by content type |
| Runs not optimized | by stopping component and command, with the most recent reason |
| Excluded sources | failing sources, each with a *Re-enable* button (`forget`) |

Filters at the top narrow everything to one session, one agent, or a period. *Reset statistics*
deletes the runs matching the filters (after a confirmation); *Stop server* stops the
background server. The footer shows the server's pid, uptime, ledger and build. Clicking a run
opens its details (`#run-42` in the address links straight to one):

- **Compare** puts the original and what the agent read side by side, with line numbers,
  synchronized scrolling and optional wrapping. Lines dropped from the original are struck
  through; lines gascoigne added (omission markers, summaries, path legends) are highlighted.
  Either side can be shown alone.
- **Steps** shows each optimizer that changed the run as a bar of what was left after it.
- **Details** lists the source key, pipeline and route, optimizer chain, stop reason, timing
  and the `gascoigne raw` command.

A run that lost over 99.5% of a large output gets a warning to check it. The optimized text is
kept next to the original (gzip, same retention), so runs recorded before that show only the
original. Viewing either here does not mark the source as failing.

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
| `POST /api/reset?session=&agent=&model=&since=&all=1` | delete recorded runs; `all=1` (no filters) also originals and failures |
| `GET /api/originals` | stored originals: count and compressed size |
| `POST /api/flush` | delete every stored original |
| `GET /api/health` | pid, build, ledger, address, start time |
| `POST /api/shutdown` | stop the server |

### Safety

The dashboard listens on `127.0.0.1` by default and shows originals, which may contain
secrets from command output. It answers only requests addressed to `localhost` or a loopback
IP (blocking DNS rebinding) and refuses cross-origin writes. Keep `-addr` on loopback.
