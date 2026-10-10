# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, explicit registry.

## Layout
```
component/  payload/  tokens/        public contracts: what component authors import
detect/  compress/  mcptool/         public libraries: content detection, text primitives, MCP tools
receiver/  filter/  exporter/        components, one package each, OTel-contrib style;
extension/                             only import public packages → can become own modules
optimizer/<family>/<name>optimizer   optimizers grouped by family (see below)
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
Well-known attrs: `cmd`, `file`, `url`, `tool`, `mime`, `query`, `session`, `raw`, `raw_id`, `route`, `raw_tokens`, `out_tokens`.
`Key()` = stable source identity (`file:` > `url:` > `cmd:` > `tool:`), used for failure history.

## Content detection: `family/dialect` (`detect/`, `filter/detectfilter`)
Every block gets a type `family/dialect` (`test/jest`, `build/tsc`, `pkg/pip`, `task/nx`), so one
payload can mix types and each part meets its family engine. Dialects are declarative `Spec`s in
`detect/<family>.go`: command patterns (`program sub sub -flag`, most specific wins, `CmdFunc` for
flag values), file globs, MIME types and a sniffer with a priority (composite runners first,
generic formats last). `Argv` unwraps wrappers and runners (`sudo`, `npx`, `uv run`,
`bundle exec`, `python -m`). Evidence order: command → file/MIME → sniffing → `doc/text`.
A generic type from command, file or MIME is refined by a spec listing it in `Refines` whose
sniffer matches (`cat deploy.yaml` with manifests → `k8s/manifest`, a saved aws response →
`cloud/aws`). `detect/matrix_test.go` requires a command example for every dialect with
command patterns and a content example (an eval fixture) for every dialect with a sniffer.
`detect.Family`/`detect.Dialect` split a type; `detect.Command` names commands for stats.

## Families
One engine per family (`optimizer/<family>/<family>optimizer`), dialects tune it. Shared logic
lives in public libs so engines and composite families can reuse it:
| Family | Engine (lib) | Dialects |
|---|---|---|
| `test` | `compress/testreport`: go parsed; others via rules (passes counted, noise and dependency frames dropped, failures/summaries kept) | go, go-json, jest, vitest, bun, mocha, deno, pytest, cargo, rspec, maven, dotnet |
| `build` | `compress/diag`: diagnostics grouped by file, identical ones across 3+ files folded, code frames capped (annotations kept), per-dialect noise/prefix rules | go, tsc, eslint, golangci, ruff, mypy, rustc, gcc, clang, javac, maven, dotnet, bundler, docker (RUN steps composed) |
| `pkg` | `compress/pkgnoise`: progress/downloads dropped, warnings folded by kind | npm, pnpm, yarn, bun, pip, uv, poetry, go-mod, cargo, os |
| `task` | sections; ok/cached collapse; failing ones via `compress/compose` | nx, turbo, make, just, gradle, compose, ci |
| `vcs` | git engine; blame runs grouped, mirrored remote branches dropped, gh lists via `compress/table` | git-status, git-log, git-diff, git-remote, git-blame, git-branch, gh-list, gh-view |
| `search` | grep engine: matches grouped by file | grep, rg, ag, git-grep |
| `fs` | ls engine (listings, tree, paths by directory, du largest, df real disks); cat: comments dropped, outlines (see Source files) | ls, tree, find, du, df, code |
| `table` | `compress/table`: parse boxed/aligned/ruled/fixed-width, constants hoisted, cells cut, rows via `compress/rows` (unhealthy statuses kept) | ps, ss, lsof, top, systemctl, docker, aligned, boxed |
| `data` | json: `compress/jsonx` crusher; data: YAML via jsonx when smaller, CSV via table, XML sibling runs sampled (failures kept), TOML lockfiles one line per entry | json, ndjson, yaml, csv, xml, toml |
| `log` | log engine: templates with counts, errors with context; structured cloud events → lines; access logs → traffic summary | app, journal, syslog, docker, kubectl, access, cloudwatch, gcp, azure |
| `doc` | markdown/rst: badges, comments, layout HTML, TOCs dropped, code fences capped; man/help: furniture and boilerplate dropped, dedented | markdown, rst, man, help (+ text: dedupe) |
| `web` | html: `compress/htmlx` → markdown; http: transport headers and TLS chatter dropped, body by type | html, http |
| `db` | results via `compress/table`, EXPLAIN pruned to costly nodes, redis keys folded, mongo docs as JSON rows | psql, mysql, sqlite, explain, redis, mongosh, migrate |
| `k8s` | `compress/yamlx` prune rules (managedFields, last-applied, defaults); lists keep unhealthy items; describe/events/helm | get, manifest, describe, events, helm |
| `infra` | plans: refresh/progress folded, known-after-apply listed, count/for_each siblings "(like above)"; ansible: quiet tasks counted, failures and recap kept | terraform, pulumi, ansible, cdk, helm-diff |
| `cloud` | JSON: pagination/metadata/etags pruned, envelopes unwrapped, items flattened so jsonx hoists leaves; YAML via yamlx; tables; s3 listings summarized | aws, aws-s3, gcp, azure |

`compress/compose` sniffs a section and calls the test/build/pkg engine: used by `task` and docker
builds. `compress/rows` samples records (errors, anomalies, change points, query, ends).
Families are what `gain -by family` reports; `gain -by dialect` splits them.

JSON arrays (`compress/jsonx`, used by `optimizer/data/jsonoptimizer`, cloud, data and web): small arrays (< `min_items`, < `min_tokens`, ≤ `max_items`)
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

## Source files (`optimizer/fs/catoptimizer`)
Agents edit by exact text, so source is shrunk only by dropping whole lines; kept lines are
byte-exact (indentation and trailing comments untouched: Python stays valid as far as shown).
Each gap is one marker in the file's comment syntax naming the original lines
(`// … comments L12-30`, `# … docstring L3-9`, `// … L50-89` for a body; `N lines` instead of
numbers for `tail`/`sed`), so an agent edits within kept runs and reads a gap back by range
(`sed -n 12,30p f`: small, passes untouched) or the whole file (`cat_mode=full`, `gascoigne raw`).
- clean (auto, 800–4000 tokens): comment runs go; a run right above code keeps its first line
  with words (doc summary), Python docstrings after `def`/`class` keep their summary line.
  Runs cheaper than a marker and directives (`//go:`, `# type:`, `noqa`, shebang, coding,
  eslint/ts pragmas) stay. Lines inside multi-line strings (backticks, triple quotes, heredocs)
  are never taken for comments.
- outline (auto, ≥ 4000 tokens): clean plus function bodies elided, signatures and closing
  braces kept as they are.
- Language-agnostic: comment syntax per extension (`//`+`/* */`, `#`, `--`, `;`, `%`), else by
  family (`detect.Lang`); unknown languages are left alone. Agents' own file-read tools are
  never touched: only `cat`/`head`/`tail` output and MCP `compress`.

## Agents (`cmd/gascoigne/agents.go`, `hook.go`)
`gascoigne wrap <agent> [args]` starts the agent with gascoigne registered for that run only
(no config files touched) and `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` set; `wrap -print <agent>`
prints the config that registers it for good; `unwrap [agent...]` (default all) removes exactly
those entries from user and project configs (claude settings + `.mcp.json` + `claude mcp remove`,
cursor hooks/mcp JSON, codex `config.toml` tables, pi extension), keeps other keys in order and
saves the original as `<file>.gascoigne-bak`; unparsable files are reported, not rewritten.
Hooks call `gascoigne hook <agent>`, which never
fails the tool call (bad input → no answer → call unchanged).
| Agent | Per run | How shell output is optimized |
|---|---|---|
| claude | `--mcp-config` + `--settings` | PreToolUse rewrites the command to `gascoigne exec -- <cmd>` (no permission decision: rules see the wrapped command) |
| codex | `-c mcp_servers…` + `-c hooks.PostToolUse…` | PostToolUse runs the output through pipeline `hook` and replaces the result (`decision: block`); codex rewrites a command only by approving it. Trust the hook once with `/hooks` |
| pi | `--extension` (written to the user cache dir) | `tool_call` rewrites the bash command; no MCP |
| cursor | — (config files only: `-print`) | preToolUse `updated_input`, `permission: allow` (shell approval stays with `beforeShellExecution`) |
Rewrites touch only the last command of a `&&` list when `detect.FromCmd` knows it; pipes,
redirects (except `2>&1`), subshells, other lists, background runs and commands that run until
stopped (`-f`/`--follow`, `--watch`, `dev`, `serve`, `start`, `up`) stay as they are, since
exec shows output only when the command exits.

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

Tables also include `steps` (per event, each optimizer that changed tokens: optimizer, family,
tokens before/after) and `events.session`.

Identity: `-session ID`, `-agent NAME`, `-model NAME` on any command (env `GASCOIGNE_SESSION`,
`GASCOIGNE_AGENT`, `GASCOIGNE_MODEL`) tag events. MCP: agent defaults to the client's name
(clientInfo), model only when given. Time: `gain -since 7d|4w|6mo|24h`.

Sessions: `-session ID` on any command (or `$GASCOIGNE_SESSION`) tags events; MCP servers use
explicit `-session`, else the MCP transport session id, else one id per server run.
Reports — `gascoigne gain` (add `-session ID`, `-all`, `-json`):
| Command | Shows |
|---|---|
| `gain` | overview: totals line, last 7 days trend (quiet days shown), by agent, by model (once recorded), by outcome (optimized/unchanged/bypassed/error with tokens and share of input), by command (`go test`, `git log`, `fetch go.dev`), by optimizer (with family), skipped / failed / unchanged with reasons, count of failing sources |
| `gain -by day\|week\|month` | trend, oldest first, bars scaled to the best period |
| `gain -by agent\|model\|session\|command\|optimizer\|family\|outcome\|reason\|route` | one grouping |
| filters, any view | `-session`, `-agent`, `-model`, `-since 7d\|4w\|6mo\|24h` (combine freely) |
| `gain -failures` | sources that bypass optimization after going bad |
Terminal output (`cmd/gascoigne/render.go`): human token counts, savings/share bars, colors by
saved % (green ≥60, yellow ≥30, red); `-color auto|always|never`, auto honors `NO_COLOR` and
TTY detection. MCP `stats` stays plain text (no ANSI/bars: they cost the agent tokens).
Every run that is not optimized records a factual reason (`events.reason`), shown per
component and command in the overview's SKIPPED / FAILED / UNCHANGED sections and in JSON:
- bypass: `raw requested: attr raw=true` or `env GASCOIGNE_RAW=1`
- minsize: `45 tokens < min_tokens 200`
- history: `<key> failed N× (max_failures M), last: <reason> at <time>; reset: gascoigne forget <key>`
- optimizer error: first line of the error; ledger failure: `save original: …`
- unchanged: optimizer note (e.g. `cat: 590 tokens < clean_min_tokens 800`) or
  `no optimizer changed it`, plus the detected types (which content lacks an optimizer)
Filters and no-op optimizers explain themselves via the `skip_reason` attr (`payload.AttrSkip`).
`gain -by reason` lists these groups alone.
Events record `command` (normalized via `detect.Command`, else MCP tool + URL host) and, when
bypassed, the `filter` that skipped them.
MCP `stats {by?, session?, format?: json}` defaults to the overview of the calling session (`session: all` for every session).

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
