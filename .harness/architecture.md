# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, explicit registry.

## Layout
```
component/  payload/  tokens/        public contracts: what component authors import
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
`payload.Payload{Blocks []Block{Kind, Text}, Attrs map[string]string}`.
Kinds: `cmd_output`, `tool_result`, `message`, `json`.
Well-known attrs: `cmd`, `file`, `tool`, `raw`, `raw_id`, `route`, `raw_tokens`, `out_tokens`.
`Key()` = stable source identity (`file:` > `cmd:` > `tool:`), used for failure history.

## Full-payload guarantee
Lives in core (`service/pipeline.go`), not in optimizers:
1. Before optimizers run, core snapshots the payload.
2. If output differs, original goes to the ledger first; `raw_id` set; stdout hint:
   `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. Ledger save fails → original returned. Optimizer errors → original returned + failure recorded.
4. Up-front opt-out: `bypass` filter (`-raw`, `GASCOIGNE_RAW=1`, attr `raw=true`).
5. Retrieval: `gascoigne raw <id>` (later MCP `get_raw`, proxy `GET /raw/<id>`).

## Ledger (sqlite, `extension/ledger`)
Tables: `events` (every trip: route, tokens, outcome), `raw` (originals, TTL-pruned),
`failures` (per-key count). Migrations embedded in `migrations/NNN_*.sql`, tracked by
`PRAGMA user_version` — add a new file, never edit an applied one.

Failure signals → `MarkFailure(key, reason)`:
- optimizer error (`error`), agent fetched raw (`raw_requested`), `gascoigne fail` (`manual`).
`history` filter bypasses a key once `count >= max_failures`; `gascoigne forget <key>` resets.

## Config
`configs/default.yaml` (embedded in binary). Lookup: `-config` > `$GASCOIGNE_CONFIG` >
`$XDG_CONFIG_HOME/gascoigne/config.yaml` > embedded. Strict decoding: unknown fields fail.
Pipelines may override `router` (proxy and CLI route differently).
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
