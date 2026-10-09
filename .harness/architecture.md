# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, explicit registry.

## Layout
```
component/  payload/  tokens/        public contracts: what component authors import
detect/  compress/  mcptool/         public libraries: content detection, text primitives, MCP tools
receiver/  filter/  optimizer/       components, one package each, OTel-contrib style;
exporter/  extension/                  only import public packages → can become own modules
internal/config  router  service     engine: YAML, routing, pipeline execution
internal/registry                    built-in component list (the "distribution")
cmd/gascoigne  configs/              CLI + embedded default config
```
`make imports` fails if a public package imports `internal/`.

## Flow
```
           ┌──────────────── pipeline (sync, request → response) ────────────────┐
input ─▶ receiver ─▶ filter* ─▶ router ─▶ optimizer chain ─▶ exporter* ─▶ output
                   (bypass, detect,          (each optimizer touches only
                    minsize, history)         blocks of its content types)
                        │ bypass                    ▲
                        └───────────────────────────┘ (skip optimizers)

core service: original snapshot ─▶ ledger (sqlite): raw blob + event ─▶ raw_id + hint
history filter ◀── ledger failures (source optimized badly → bypass next time)
```

| Kind | Contract | Role |
|---|---|---|
| receiver | `Run(ctx)`, gets `Settings.Pipeline` | turn input into `Payload`, return result to caller |
| filter | `Filter(ctx, p) (Decision, error)` | `Continue` or `Bypass`; may tag attrs |
| router | core, `router.Config` | first matching route picks optimizer chain, else `default` |
| optimizer | `Optimize(ctx, p) error` | rewrite payload in place, deterministic |
| exporter | `Export(ctx, p) error` | write / forward the result |
| extension | lifecycle only | shared services; `ledger` implements `component.Ledger` |

All embed lifecycle `Start(ctx, Host)` / `Shutdown(ctx)` (`component.Nop` for none).
Start order: extensions → exporters → optimizers → filters → receivers. Shutdown reversed.

## Payload
`payload.Payload{Blocks []Block{Kind, Text, Type}, Attrs map[string]string}`.
Kinds (where it came from): `cmd_output`, `tool_result`, `message`, `json`.
Type (what it is): detected content type, per block — see below.
Well-known attrs: `cmd`, `file`, `url`, `tool`, `mime`, `query`, `raw`, `raw_id`, `route`, `raw_tokens`, `out_tokens`.
`Key()` = stable source identity (`file:` > `url:` > `cmd:` > `tool:`), used for failure history.

## Content detection (`detect/`, `filter/detectfilter`)
The `detect` filter sets `Block.Type` on every block, so one payload can mix types (several tool
results in one request) and each part meets the right optimizer. Order of evidence:
1. command hint (`cmd` attr): `go test`, `git status|log|diff|show|push`, `npm|pnpm|yarn`, `bun`,
   `nx`/`npx nx`, `tsc`, `jest|vitest`, `grep|rg|ag`, `ls`, `cat|head|tail <file>`;
2. file name or MIME (`file`, `mime` attrs);
3. content sniffing: json/ndjson/json streams, html, diff, go test(-json)/build, git status/log,
   tsc, bun, nx, jest/vitest, npm, grep results, `ls -l`, logs, source code, else `text`.
Types are constants in `detect` (`detect.GoTest`, `detect.JSON`, …). New format = new constant +
cmd mapping and/or sniffer + table test, then an optimizer that selects it.

## Built-in components
| Kind | Components |
|---|---|
| receiver | `stdin`, `exec` (`gascoigne exec -- cmd`), `mcp` (server: fetch, compress, retrieve, stats), `mcp_proxy` (wraps another MCP server) |
| filter | `bypass`, `detect`, `minsize`, `history` |
| optimizer | `go`, `git`, `grep`, `ls`, `json`, `log`, `cat`, `npm`, `bun`, `nx`, `html`, `passthrough` |
| exporter | `stdout` |
| extension | `ledger` |

JSON arrays (`optimizer/jsonoptimizer`): small arrays (< `min_items`, < `min_tokens`, ≤ `max_items`)
are never sampled. Otherwise lossless first — similar objects become a table (`_cols/_rows`,
constant columns in `_same`, shared prefixes/suffixes in `_affix`), kept when it saves
≥ `lossless_min_saving` and fits `lossless_max_tokens`. Else scored sampling to ~`max_items`:
errors always (error fields, bad statuses, 4xx/5xx, keys or enum values few items have),
then by score anomalies (> `anomaly_std` σ), change points (numeric jumps, enum flips) and
BM25 matches of the `query` attr (`-attr query=...`, MCP `query` param), then head/tail
shares, then an even sample; identical items collapse. Gaps become `… N items omitted`.

Optimizers self-select by `Block.Type` through `payload.Rewrite`, so the default router sends
every payload through one chain of all optimizers; routes only override per command/content.

## MCP
- `gascoigne mcp`: stdio server. `fetch` turns web pages into markdown and compresses API JSON
  (http/https only, timeout, size cap) — the way to read web data cheaply. `compress` takes any
  text with optional `cmd`/`file` hints. `retrieve id` returns originals and marks the source as
  badly optimized. `stats` reports savings.
- `gascoigne mcp-proxy -- <server cmd>`: mirrors a downstream server's tools, optimizes their text
  results, adds `gascoigne_retrieve`. Non-text content passes untouched.
- MCP pipelines have no exporters: stdout is the protocol channel.

## Full-payload guarantee
Lives in core (`service/pipeline.go`), not in optimizers:
1. Before optimizers run, core snapshots the payload.
2. If output differs, original goes to the ledger first; `raw_id` set; stdout hint:
   `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. Ledger save fails → original returned. Optimizer errors → original returned + failure recorded.
4. Up-front opt-out: `bypass` filter (`-raw`, `GASCOIGNE_RAW=1`, attr `raw=true`).
5. Retrieval: `gascoigne raw <id>`, MCP `retrieve` / `gascoigne_retrieve` (later HTTP proxy `GET /raw/<id>`).

## Ledger (gorm + sqlite, `extension/ledger`)
Tables: `events` (every trip: route, tokens, outcome), `raw` (originals, TTL-pruned),
`failures` (per-key count). Gorm models in `models.go`; `AutoMigrate` on Start adds new
tables/columns/indexes. Renames, drops or type changes need an explicit migration step — ask first.
Pure-Go driver (`gormlite`), WAL mode, busy timeout 5s.

Failure signals → `MarkFailure(key, reason)`:
- optimizer error (`error`), agent fetched raw (`raw_requested`), `gascoigne fail` (`manual`).
`history` filter bypasses a key once `count >= max_failures`; `gascoigne forget <key>` resets.

## Config
`configs/default.yaml` (embedded in binary). Lookup: `-config` > `$GASCOIGNE_CONFIG` >
`$XDG_CONFIG_HOME/gascoigne/config.yaml` > embedded. Strict decoding: unknown fields fail.
Pipelines may override `router` (proxy and CLI route differently).

Per-run overrides (`internal/config/override.go`), so each agent can tune optimization without
its own config file — flags on any command, or env set once in the agent's environment:
| Flag | Env | Effect |
|---|---|---|
| `-set path=yaml` (repeatable) | `GASCOIGNE_SET` (`;`-separated, applied before flags) | assign any config value: `-set optimizers.grep.max_per_file=50`, `-set router.default=[go,git]` |
| `-only a,b` | `GASCOIGNE_ONLY` | keep only these optimizers in every chain |
| `-disable a,b` | `GASCOIGNE_DISABLE` | drop these optimizers from every chain |
Flags win over env. `gascoigne config [flags]` prints the effective config. Per payload,
`-attr k=v` still sets attrs (e.g. `cat_mode=full`).
Optimizers configured in a pipeline's router require a ledger extension.

## Adding a component
See skill `add-component`. Package shape:
```
<kind>/<name><kind>/
  factory.go   NewFactory() via component.NewFactory, const typ
  config.go    Config struct with yaml tags + Validate() if constraints
  <name>.go    implementation (package doc here)
  <name>_test.go
```
Then one line in `internal/registry/registry.go`.
