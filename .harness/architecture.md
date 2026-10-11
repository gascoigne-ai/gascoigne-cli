# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, explicit registry.

## Layout
```
component/  payload/  tokens/        public contracts: what component authors import
detect/  compress/  mcptool/         public libraries: content detection, text primitives, MCP tools
lazyre/                              package-level regexps compiled on first use (init stays ~1 ms)
version/                             release number + git commit from build info (CLI, MCP, health)
cost/                                tokenizer families (factor vs tokens.Estimate), prices, agent context model
receiver/  filter/  exporter/        components, one package each, OTel-contrib style (receivers:
                                       stdin, exec, hook, mcp, mcp_proxy, httpproxy);
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
`detect.Family`/`detect.Dialect` split a type; `detect.Command` names commands for stats (leading
`cd`/`export` steps of a list skipped). The detect filter's `edit_safe` types config-sized files
read from disk (`detect.ReadsFile`: cat/head/tail/sed) of data/k8s/cloud/infra as `fs/code`, so a
config read to be edited keeps its syntax.

## Families
One engine per family (`optimizer/<family>/<family>optimizer`), dialects tune it. Shared logic
lives in public libs so engines and composite families can reuse it:
| Family | Engine (lib) | Dialects |
|---|---|---|
| `test` | `compress/testreport`: go parsed; others via rules (passes counted, noise and dependency frames dropped, failures/summaries kept) | go, go-json, jest, vitest, bun, mocha, deno, pytest, cargo, rspec, maven, dotnet |
| `build` | watch output (`compress/watch`) keeps the last cycle + earlier result lines; `compress/diag`: diagnostics grouped by file, identical ones across 3+ files folded, code frames capped (annotations kept), per-dialect noise/prefix rules | go, tsc, eslint, golangci, ruff, mypy, rustc, gcc, clang, javac, maven, dotnet, bundler, docker (RUN steps composed) |
| `pkg` | `compress/pkgnoise`: progress/downloads dropped, warnings folded by kind | npm, pnpm, yarn, bun, pip, uv, poetry, go-mod, cargo, os |
| `task` | sections; ok/cached collapse; failing ones via `compress/compose` | nx, turbo, make, just, gradle, compose, ci |
| `vcs` | git engine; diffs: in-line edits as word diffs (`~[-old-]{+new+}`, only when every pair of a k:k block qualifies), lockfiles/generated files folded to counts; removed blocks re-added verbatim elsewhere → `-… N lines moved to file:L` (`moves.go`); log keeps `--stat`/name lists; blame runs grouped, mirrored remote branches dropped, gh lists via `compress/table` | git-status, git-log, git-diff, git-remote, git-blame, git-branch, gh-list, gh-view |
| `search` | grep engine: matches grouped by file (with or without line numbers), identical lines in a file merged (`3,9: text`, `2× text`); with a `query` the per-file cap keeps the best BM25 matches | grep, rg, ag, git-grep |
| `fs` | ls engine (listings, tree, paths by directory, du largest, df real disks); cat: comments dropped, outlines (see Source files) | ls, tree, find, du, df, code |
| `table` | `compress/table`: parse boxed/aligned/ruled/fixed-width, constants hoisted, cells cut, rows via `compress/rows` (unhealthy statuses kept) | ps, ss, lsof, top, systemctl, docker, aligned, boxed |
| `data` | json: `compress/jsonx` crusher; data: YAML via jsonx when smaller, CSV via table, XML sibling runs sampled (failures kept), TOML lockfiles one line per entry | json, ndjson, yaml, csv, xml, toml |
| `log` | log engine: templates with counts, errors with context, numeric outliers kept (≥ 10× the template's median at a place, 8+ lines); container prefixes (kubectl `--prefix`, compose) cut before and put back as labels (`A+B| `, legend); structured cloud events → lines; access logs → traffic summary | app, journal, syslog, docker, kubectl, access, cloudwatch, gcp, azure |
| `doc` | markdown/rst: badges, comments, layout HTML, TOCs dropped, code fences capped; markdown over `max_tokens` keeps whole sections (intro, strong BM25 matches of `query` (≥ half the best; versions like `go1.27` are terms), then page order; when whole sections use under half the budget, the first left out keeps its head), dropped ones leave top-level headings + `… N lines omitted`; man/help: furniture and boilerplate dropped, dedented; help with a `query`: option entries scored by BM25, ≥ a third of the best kept | markdown, rst, man, help (+ text: dedupe) |
| `web` | html: `compress/htmlx` → markdown (root: the `<main>`/`<article>` with the most visible text when ≥ half the page's, not one holding articles or named a comment; else the page); http: transport headers and TLS chatter dropped, body by type | html, http |
| `db` | results via `compress/table`, EXPLAIN pruned to costly nodes, redis keys folded, mongo docs as JSON rows | psql, mysql, sqlite, explain, redis, mongosh, migrate |
| `k8s` | `compress/yamlx` prune rules (managedFields, last-applied, defaults); lists keep unhealthy items; describe/events/helm | get, manifest, describe, events, helm |
| `infra` | plans: refresh/progress folded, known-after-apply listed, count/for_each siblings "(like above)"; ansible: quiet tasks counted, failures and recap kept | terraform, pulumi, ansible, cdk, helm-diff |
| `cloud` | JSON: pagination/metadata/etags pruned, envelopes unwrapped, items flattened so jsonx hoists leaves; YAML via yamlx; tables; s3 listings summarized | aws, aws-s3, gcp, azure |

`embedjson` (`optimizer/core/embedjsonoptimizer`, first in the default chain): JSON values with record
arrays inside log/test/build/task/text blocks (`jsonx.Spans`) crushed in place by jsonx; text around byte-exact.
`dense` (`optimizer/core/denseoptimizer`, after the family engines): long space-free lines
(minified code, base64, data URIs) keep head/tail bytes around `…[N bytes dense]…` once a block
holds `min_total` such bytes; JSON lines and fs/code read from disk (file attr, `detect.ReadsFile`) untouched.
`paths` (`optimizer/core/pathsoptimizer`, after `dense`): long path prefixes repeated
in test/build/search/log/task blocks → `§1/file.go`, legend `[paths: §1=…]` on top of the first
such block; greedy by bytes saved, kept only when ≥ `min_saving` tokens; idempotent; never fs/data.
`truncate` (`optimizer/core/truncateoptimizer`, after `paths`): blocks still > `max_tokens` keep
head/tail token budgets + `max_errors` error lines from the middle, long lines cut; `skip`
fs/code, data, k8s, cloud.
`compress/probe` measures retention: error lines, artifacts (paths, URLs, hashes, UUIDs) and
`key=number` pairs of an original still readable in the output (regrouping, path legends and
short hashes count as kept). `gascoigne eval` shows lost counts per case and pins them with out
tokens in `testdata/eval/baseline.tsv` (any change fails until `-update`); `gascoigne probe`
runs it over ledger originals vs stored outputs (proxy folds skipped), per family + worst runs.
`compress/compose` sniffs a section and calls the test/build/pkg engine: used by `task` and docker
builds. `compress/rows` samples records (errors, anomalies, change points, query, ends).
Families are what `gain -by family` reports; `gain -by dialect` splits them.

JSON arrays (`compress/jsonx`, used by `optimizer/data/jsonoptimizer`, cloud, data and web): small arrays (< `min_items`, < `min_tokens`, ≤ `max_items`)
are never sampled. Otherwise lossless first — similar objects become a table (`_cols/_rows`,
constant columns in `_same`, shared prefixes/suffixes in `_affix`; items with nested objects are
also tried flattened to dotted columns via `jsonx.Flatten`, the smaller table wins), kept when it saves
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

## LLM API proxy (`receiver/httpproxyreceiver`, `cmd/gascoigne/llmproxy.go`)
Served by the background server under `/v1/` (one process, one port: base URL
`http://127.0.0.1:7878`); the receiver is an `http.Handler` (`Run` is a no-op), built from the
user's config (`receivers.httpproxy`, pipeline `proxy` via `optimizerService`, payloads one at a
time under the warm services' lock). Anthropic `/v1/messages*` (by path or
`anthropic-version`/`x-api-key` headers), OpenAI `/chat/completions` and `/responses`, Gemini
`/v1beta/models/<m>:generateContent|:streamGenerateContent` (also mounted at `/v1beta/`; by path or
`x-goog-api-key`; `functionResponse.response.output|content` answers the call with its id, else the
oldest open call of its name; model from the path); other paths
and every header pass through (`anthropic-beta` carries the OAuth capability, so a claude.ai login
keeps working), responses stream through (`FlushInterval -1`). Per tool result, one payload with
attrs from the matching call (`attrsOf`: built-in tools described as the shell command that
prints the same thing — Read → `cat <path>` or `sed -n <offset>,$p` for slices, Grep → `rg -n|-l|-c`,
Glob/list → `find`; `query`/`prompt` → query attr, url, tool; model from the body, agent from the
User-Agent); `skip` tools, `is_error`, hinted output untouched. Read's
`N\t` prefixes are split off, the text optimized like `cat`, and kept lines get their original
numbers back (`renumber`). Prompt cache: results cached by content hash (LRU `cache_entries`), so
every later request sends the same bytes; optimizers are deterministic, so a restart recomputes
them. Fold (`fold.go`, per request): after shortening, runs of ≥ `fold_min_lines` lines /
`fold_min_chars` bytes an earlier result of the request shows (Read lines matched by text with a
uniform number shift) → one pointer (`[↑ L13-18 = L10-15 of earlier Read a.go]`); recomputed each
request (a result depends only on earlier ones: prefix-stable, no dangling pointers when clients
clear old results); original via `SaveRaw` (content-addressed id) + event (route `fold`), saves
memoized; needs `ledger` (looked up in `Start`). Usage (`usage.go`): 2xx response bodies pass through a meter (SSE `data:` lines mentioning usage
decoded, JSON bodies ≤ 8 MB at the end); per API reader, fields max-merged across events, cached
input split out (OpenAI/Gemini count it in input); on close one `component.Usage` → the ledger's
optional `component.UsageRecorder` (table `usage`, no session), `gain` section "api usage" priced
by `cost.Lookup`, JSON `api_usage`; `reset` filters apply. Any parse problem → body forwarded unchanged; `X-Gascoigne-Raw: 1` → untouched. On a new
build the server waits until no proxied request is in flight (`waitIdle`, ≤ 10 min) before
restarting.
`gascoigne replay` (`cmd/gascoigne/replay.go`) rebuilds Claude Code transcripts (`~/.claude/projects`,
newest first; lines of one role merged, sidechains skipped, compaction restarts) into one request
per model turn after tool results, sends them through `llmProxy` against an in-process upstream
with a throwaway ledger, and reports tokens per tool, message input cost as sent and as forwarded
(`cost` prices: the prefix the previous request sent identically read from the cache, the rest
written), plus how many re-sent messages the proxy
forwarded byte-identical (canonical JSON).
Registration (server started first, `serverUp`): wrap claude `ANTHROPIC_BASE_URL` (unless set: user gateway), codex with
`OPENAI_API_KEY` a `model_providers.gascoigne` (`wire_api=responses`), opencode provider
`baseURL`s, gemini with `GEMINI_API_KEY` `GOOGLE_GEMINI_BASE_URL` (a Google login stays direct); `install -proxy claude` writes the env for good (SessionStart starts the server), unwrap removes it.

## Source files (`optimizer/fs/catoptimizer`)
Agents edit by exact text, so source is shrunk only by dropping whole lines; kept lines are
byte-exact (indentation and trailing comments untouched: Python stays valid as far as shown).
Each gap is one marker in the file's comment syntax naming the original lines
(`// … comments L12-30`, `# … docstring L3-9`, `// … L50-89` for a body; `N lines` instead of
numbers for `tail`/`sed`), so an agent edits within kept runs and reads a gap back by range
(`sed -n 12,30p f`: small, passes untouched) or the whole file (`cat_mode=full`, `gascoigne raw`).
- clean (auto, 800–4000 tokens): comment runs go; a run right above code (or one blank line above
  code at its indent, unless before any code) keeps its first line
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
(no config files touched) and `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` set; `install [-g] [agent...]`
writes the config that registers it for good into project files (`-g`: user files; existing
gascoigne entries replaced, JSON merged, backup kept, `install.go`), `wrap -print <agent>` prints it; `unwrap [agent...]` (default all) removes exactly
those entries from user and project configs (claude settings + `.mcp.json` + `claude mcp remove`,
cursor hooks/mcp JSON, codex `config.toml` tables, pi extension), keeps other keys in order and
saves the original as `<file>.gascoigne-bak`; unparsable files are reported, not rewritten.
Agents get a usage guide (`guide.go`): claude a SessionStart hook (`additionalContext`), codex
`-c developer_instructions` per run or an `AGENTS.md` marked block, cursor a project rule, pi an
`AGENTS.md` block; unwrap removes them.
Other stdio MCP servers start through `gascoigne mcp-proxy -- <cmd>` (`proxy.go`): wrap passes
same-named copies (claude `--mcp-config` beats configured scopes for the run, approved/enabled
ones only; codex `-c` overrides), install rewrites `.mcp.json`/`~/.claude.json`/codex tables/
cursor `mcp.json`, unwrap restores from the kept args; remote servers stay direct.
Results the rewrite cannot reach are replaced after the tool ran (`posthook.go`): claude
PostToolUse `updatedToolOutput` in the tool's shape (Bash stdout, Grep content mode, `mcp__*`),
codex PostToolUse `decision: block` + `reason` (`^Bash$|^mcp__.*`), gemini AfterTool
`decision: deny` + `reason` (shell, `mcp_*`). Hinted output (`hintRe`) and gascoigne's own MCP
tools are skipped; Read is never replaced (Claude numbers its lines). `install claude` adds the
same deny rules as wrap (Grep, Glob, WebFetch) and records them in `$XDG_STATE_HOME/gascoigne/
denies.json`; unwrap removes only recorded ones (`denies.go`).
Hooks call `gascoigne hook <agent>`, which never
fails the tool call (bad input → no answer → call unchanged).
| Agent | Per run | How shell output is optimized |
|---|---|---|
| claude | `--mcp-config` + `--settings` (denies Grep/Glob so searches run through Bash, WebFetch so pages go through gascoigne fetch; Read stays, Edit needs it; WebSearch stays) | PreToolUse rewrites the command to `gascoigne exec -- <cmd>` (no permission decision: rules see the wrapped command) |
| codex | `-c mcp_servers…` + `-c hooks.PostToolUse…` | PostToolUse runs the output through pipeline `hook` and replaces the result (`decision: block`); codex rewrites a command only by approving it. Trust the hook once with `/hooks` |
| pi | `--extension` (written to the user cache dir) | `tool_call` rewrites the bash command; no MCP |
| gemini | env `GEMINI_CLI_SYSTEM_SETTINGS_PATH` → run settings in the cache dir (real system settings merged in, user/project stdio servers proxied) | BeforeTool `hookSpecificOutput.tool_input` rewrites `run_shell_command` (no decision: approval stays); SessionStart adds the guide (`gemini.go`) |
| opencode | env `OPENCODE_CONFIG_DIR` (plugin in the cache dir) + `OPENCODE_CONFIG_CONTENT` (`mcp`, proxied servers, `instructions` guide unless the user has some) | plugin `tool.execute.before` rewrites `output.args.command` via `gascoigne hook opencode` (`opencode.go`) |
| cursor | — (config files only: `-print`) | preToolUse `updated_input`, `permission: allow` (shell approval stays with `beforeShellExecution`) |
Rewritten commands carry `-agent <name>` and `-session <hook session_id>` when
`GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` are unset (installed hooks; `wrap` sets the env, so its
commands stay short): `execPrefix` in `hook.go`.
Rewrites wrap every command of a `&&`/`;` list except builtins, quiet, interactive and REPL
commands (`passthrough` in `hook.go`; unknown programs are detected by content) (lines with loops,
conditionals or `{ }` groups stay as they are), and
may keep trailing `| head`/`| tail` stages (they then cut optimized text; other filters would
search it, so other pipes stay). Before `head`, exec gets `-attr hint=top`: the stdout exporter
prints the hint first so the cut keeps it. Redirects (except `2>&1`), subshells, `||`, background runs,
commands running gascoigne itself and commands that run until stopped (`-f`/`--follow`,
`--watch`, `dev`, `serve`, `start`, `up`, plain `vite`) stay as they are, since exec shows output only when
the command exits — except build tools in watch mode (`watched`): `exec -watch` processes each
cycle when `watch.End` sees its result line, background runs included.

## Full-payload guarantee
Lives in core (`service/pipeline.go`), not in optimizers:
1. Before optimizers run, core snapshots the payload.
2. Output saving fewer than `minSaving` (20) tokens, the cost of the retrieve hint, → original
   kept, outcome unchanged.
   If output differs, original goes to the ledger first; `raw_id` set; stdout hint:
   `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. Ledger save fails → original returned. Optimizer errors → original returned + failure recorded.
4. Up-front opt-out: `bypass` filter (`-raw`, `GASCOIGNE_RAW=1`, attr `raw=true`).
5. Retrieval: `gascoigne raw <id>`, MCP `retrieve` / `gascoigne_retrieve` (later HTTP proxy `GET /raw/<id>`).

## Ledger (gorm + sqlite, `extension/ledger`)
Tables: `events` (every trip: route, tokens, outcome), `raw` (originals gzip-compressed, plain
JSON rows from before still read; TTL-pruned, `gascoigne flush [-older]` / `POST /api/flush` delete and VACUUM),
`failures` (per-key count). Gorm models in `models.go`; `AutoMigrate` on Start adds new
tables/columns/indexes. Renames, drops or type changes need an explicit migration step — ask first.
Pure-Go driver (`gormlite`), WAL mode, busy timeout 5s.

Tables also include `steps` (per event, each optimizer that changed tokens: optimizer, family,
tokens before/after), `events.session` and `usage` (per proxied API request: agent, model, input,
cache read/write, output tokens as the provider billed them).

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
Cost (`cost/`, `mcptool.BuildCosts`, docs/cost.md): saved tokens × tokenizer factor (claude 1.58
measured from transcripts, legacy 1.25, gemini 0.79, o200k 1) × (first read: cache write or input
+ `rereads` (measured median 110 later requests in context) × cache read, 0.8% misses at write
price), per agent and model; runs without a model use `cost.AgentModels` (marked assumed). Hooks
record the model from the tail of `transcript_path` (`transcriptModel`, `-model` in the rewrite).
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
Background server (`cmd/gascoigne/server.go`): one per user on `$GASCOIGNE_SERVER_ADDR`
(default `127.0.0.1:7878`; the port is the lock), started detached (setsid, log in the user cache
dir) by commands with `common.server` set (`exec`, `serve` for mcp/mcp-proxy, the codex hook's
`optimize`) and by `wrap`, when `/api/health` does not answer. Health reports pid, build (binary
path+size+mtime hash), binary mtime and ledger path: an older build is replaced (stop + spawn),
a newer one kept, another ledger left alone. The server stats its binary every 2 s; a new build
seen twice in a row → shut down, close the ledger, spawn the new binary (`watchBinary`). Idle exit
(`idle.go`): requests in flight and the last one's end are tracked (`/api/health` excluded; an
open stream counts), after `-idle`/`GASCOIGNE_SERVER_IDLE` (30m, 0 never) the server shuts down;
commands start it again (`ensureServer`), and agents' UserPromptSubmit hooks (claude, codex;
opencode `chat.message`) bring it back before the next API request reaches the proxy. Server mode (`remote.go`): `exec`, `run` and the codex hook (`common.useServer`)
build only their pipeline's receivers and exporters (`service.Options.Remote`) and POST the
payload to `/api/optimize`, where a warm service per config hash (defaults+user file and
overrides; receivers/exporters stripped, ≤ 8 kept, one payload at a time) runs
`Pipeline.Optimize`. Same build and ledger path required (409 otherwise); refused, 409 or a
broken server → the full pipeline is built locally for that payload (which also starts/replaces
the server); pipeline errors (422) are returned. Runs with other `GASCOIGNE_*` env (raw,
cat mode, ledger) stay local; a spawned server drops the spawner's `GASCOIGNE_*` env except
ledger/config/addr. Package-level patterns use `lazyre` (`TestPatternsCompile` runs
`lazyre.Check`), so a served run never compiles them. `server run|start|stop|status`, `dashboard` =
`server start -open` (opens a browser unless no display; `-open=false`), `GASCOIGNE_SERVER=off`. `gascoigne reset` / `POST /api/reset` → `ledger.Reset`:
events+steps by filters (flags only, never env); `-all` also raw and failures, no filters.
The server (`dashboard.go`, one embedded `dashboard.html`, no build step) serves the ledger live (version in the header, the logo moon's phase = savings rate; tables in equal-height boxes, rows rendered as they scroll; deleting data (reset, clear originals) asks twice): totals, days, outcomes, commands,
optimizers, dialects, skip/fail/unchanged reasons, failing sources (forget button) and a feed of
runs over server-sent events (`/api/stream`, polls the ledger since other processes write it;
run details compare original and optimized side by side (`/api/raw/{id}`, `/api/out/{id}`: the
optimized text is `Event.Output`, kept gzip in `raw.output`), `#run-ID` deep links;
`ledger.Recent`). A run opens its steps and original; viewing an original there does not mark the
source as failing. Loopback Host names only (DNS rebinding), cross-origin writes refused.

Failure signals → `MarkFailure(key, reason)`:
- optimizer error (`error`), agent fetched raw (`raw_requested`), `gascoigne fail` (`manual`).
`history` filter bypasses a key once `count >= max_failures`; `gascoigne forget <key>` resets.

## Config
`configs/default.yaml` (embedded in binary) holds every default; the first config file found
(`-config` > `$GASCOIGNE_CONFIG` > `$XDG_CONFIG_HOME/gascoigne/config.yaml`) is laid over it
(`cmd/gascoigne/userconfig.go` `overlay`: maps merge, values and lists replace, empty keys keep
the default). With neither flag nor env, first run writes `configs/user.yaml` there: every
setting of every component commented out (`##` docs, `#` settings). Tests keep it honest: the
template uncommented equals the defaults, and every `Config` field of every used component
appears in it (a new field fails until documented). Strict decoding: unknown fields fail.
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
