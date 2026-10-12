# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, an explicit registry.

Each `##` section stands alone; read only the one for the area you touch.

## Layout

```
component/  payload/  tokens/        public contracts: what component authors import
detect/  compress/  mcptool/         public libraries: content detection, text primitives, MCP tools
lazyre/                              package-level regexps compiled on first use (init stays ~1 ms)
version/                             release number + git commit from build info (CLI, MCP, health)
tokenizer/                           exact counts: o200k BPE in Go, Claude via count_tokens
cost/                                tokenizer families, prices, agent context model
receiver/  filter/  exporter/        components, one package each (OTel-contrib style)
extension/
optimizer/<family>/<name>optimizer   optimizers grouped by family (see Families)
internal/config  router  service     engine: YAML, routing, pipeline execution
internal/registry                    built-in component list (the "distribution")
cmd/gascoigne  configs/              CLI + embedded default config
```

- Receivers: stdin, exec, hook, mcp, mcp_proxy, httpproxy.
- Component packages import only public packages, so each can become its own module.
  `make imports` fails if a public package imports `internal/`.
- `tokenizer/` downloads the o200k vocabulary once and pins its hash.
- `cost/` relates tokenizer families to `tokens.Estimate` by a factor.

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
| receiver | `Run(ctx)`, gets `Settings.Pipeline` | turn input into `Payload`, return the result to the caller |
| filter | `Filter(ctx, p) (Decision, error)` | `Continue` or `Bypass`; may tag attrs |
| router | core, `router.Config` | first matching route picks the optimizer chain, else `default` |
| optimizer | `Optimize(ctx, p) error` | rewrite the payload in place, deterministically |
| exporter | `Export(ctx, p) error` | write or forward the result |
| extension | lifecycle only | shared services; `ledger` implements `component.Ledger` |

- Every component embeds the lifecycle `Start(ctx, Host)` / `Shutdown(ctx)` (`component.Nop` for none).
- Start order: extensions → exporters → optimizers → filters → receivers. Shutdown is reversed.
- Optimizers self-select by `Block.Type` through `payload.Rewrite`. The default router therefore
  sends every payload through one chain of all optimizers; routes only override per command or
  content.

## Payload

`payload.Payload{Blocks []Block{Kind, Text, Type}, Attrs map[string]string}`

- **Kind**, where a block came from: `cmd_output`, `tool_result`, `message`, `json`.
- **Type**, what it is: the detected content type, per block (see Content detection).
- **Well-known attrs**: `cmd`, `file`, `url`, `tool`, `mime`, `query`, `session`, `raw`, `raw_id`,
  `route`, `raw_tokens`, `out_tokens`.
- **`Key()`**: stable source identity (`file:` > `url:` > `cmd:` > `tool:`), used for failure history.

## Content detection: `family/dialect` (`detect/`, `filter/detectfilter`)

Every block gets a type `family/dialect` (`test/jest`, `build/tsc`, `pkg/pip`, `task/nx`). One
payload can mix types, and each part meets its own family engine.

### Dialect specs

Dialects are declarative `Spec`s in `detect/<family>.go`:
- command patterns: `program sub sub -flag`, the most specific wins; `CmdFunc` for flag values
- file globs and MIME types
- a sniffer with a priority: composite runners first, generic formats last

`Argv` unwraps wrappers and runners (`sudo`, `npx`, `uv run`, `bundle exec`, `python -m`).

### Evidence order

command → file/MIME → sniffing → `doc/text`.

A generic type from command, file or MIME is refined by a spec that lists it in `Refines` and
whose sniffer matches: `cat deploy.yaml` with manifests → `k8s/manifest`, a saved aws response →
`cloud/aws`.

### Helpers and rules

- `detect/matrix_test.go` requires a command example for every dialect with command patterns,
  and a content example (an eval fixture) for every dialect with a sniffer.
- `detect.Family` / `detect.Dialect` split a type.
- `detect.Command` names commands for stats; leading `cd`/`export` steps of a list are skipped.
- `edit_safe` (detect filter): config-sized data/k8s/cloud/infra files read from disk
  (`detect.ReadsFile`: cat/head/tail/sed) are typed `fs/code`, so a config read to be edited
  keeps its syntax.

## Families

One engine per family (`optimizer/<family>/<family>optimizer`); dialects tune it. Shared logic
lives in public libs so engines and composite families can reuse it. Families are what
`gain -by family` reports; `gain -by dialect` splits them.

### Family engines

| Family | Engine (lib) | Dialects |
|---|---|---|
| `test` | `compress/testreport`: go parsed, others via rules. Passes counted, noise and dependency frames dropped, failures and summaries kept | go, go-json, jest, vitest, bun, mocha, deno, pytest, cargo, rspec, maven, dotnet |
| `build` | `compress/watch`: watch output keeps the last cycle + earlier result lines. `compress/diag`: diagnostics grouped by file, identical ones across 3+ files folded, code frames capped (annotations kept), per-dialect noise/prefix rules | go, tsc, eslint, golangci, ruff, mypy, rustc, gcc, clang, javac, maven, dotnet, bundler, docker (RUN steps composed) |
| `pkg` | `compress/pkgnoise`: progress and downloads dropped, warnings folded by kind | npm, pnpm, yarn, bun, pip, uv, poetry, go-mod, cargo, os |
| `task` | sections; ok/cached ones collapse; failing ones via `compress/compose` | nx, turbo, make, just, gradle, compose, ci |
| `vcs` | git engine (see below) | git-status, git-log, git-diff, git-remote, git-blame, git-branch, gh-list, gh-view |
| `search` | grep engine: matches grouped by file (with or without line numbers), identical lines in a file merged (`3,9: text`, `2× text`); with a `query`, the per-file cap keeps the best BM25 matches | grep, rg, ag, git-grep |
| `fs` | ls engine: listings, tree, paths by directory, du largest, df real disks. cat: comments dropped, outlines (see Source files) | ls, tree, find, du, df, code |
| `table` | `compress/table`: parses boxed/aligned/ruled/fixed-width tables, hoists constants, cuts cells; rows via `compress/rows` (unhealthy statuses kept) | ps, ss, lsof, top, systemctl, docker, aligned, boxed |
| `data` | json: `compress/jsonx` crusher. YAML via jsonx when smaller, CSV via table, XML sibling runs sampled (failures kept), TOML lockfiles one line per entry | json, ndjson, yaml, csv, xml, toml |
| `log` | log engine (see below) | app, journal, syslog, docker, kubectl, access, cloudwatch, gcp, azure |
| `doc` | markdown/rst and man/help (see below) | markdown, rst, man, help (+ text: dedupe) |
| `web` | html: `compress/htmlx` → markdown (see below). http: transport headers and TLS chatter dropped, body by type | html, http |
| `db` | results via `compress/table`, EXPLAIN pruned to costly nodes, redis keys folded, mongo docs as JSON rows | psql, mysql, sqlite, explain, redis, mongosh, migrate |
| `k8s` | `compress/yamlx` prune rules (managedFields, last-applied, defaults); lists keep unhealthy items; describe/events/helm | get, manifest, describe, events, helm |
| `infra` | plans: refresh/progress folded, known-after-apply listed, count/for_each siblings "(like above)". ansible: quiet tasks counted, failures and recap kept | terraform, pulumi, ansible, cdk, helm-diff |
| `cloud` | JSON: pagination/metadata/etags pruned, envelopes unwrapped, items flattened so jsonx hoists leaves. YAML via yamlx; tables; s3 listings summarized | aws, aws-s3, gcp, azure |

### Engine details

**vcs (git engine)**
- Diffs: in-line edits as word diffs (`~[-old-]{+new+}`), only when every pair of a k:k block
  qualifies. Lockfiles and generated files fold to counts.
- Blocks removed and re-added verbatim elsewhere → `-… N lines moved to file:L` (`moves.go`).
- Log keeps `--stat` and name lists. Blame runs are grouped. Mirrored remote branches are dropped.
- gh lists go through `compress/table`.

**log**
- Lines become templates with counts; errors keep their context.
- Numeric outliers are kept: ≥ 10× the template's median at a place, for templates of 8+ lines.
- Container prefixes (kubectl `--prefix`, compose) are cut before and put back as labels
  (`A+B| `, with a legend).
- Structured cloud events → lines. Access logs → a traffic summary.

**doc**
- markdown/rst: badges, comments, layout HTML and TOCs dropped; code fences capped.
- Markdown over `max_tokens` keeps whole sections in this order: intro, strong BM25 matches of
  `query` (≥ half the best; versions like `go1.27` are terms), then page order. When whole
  sections use under half the budget, the first section left out keeps its head. Dropped
  sections leave their top-level headings + `… N lines omitted`.
- man/help: furniture and boilerplate dropped, dedented. Help with a `query`: option entries are
  scored by BM25 and those ≥ a third of the best are kept.

**web (html)**
- The root is the `<main>`/`<article>` with the most visible text, when that is ≥ half the page's
  and it neither holds articles nor is named a comment; else the whole page.

### Core optimizers (in default chain order)

1. **`embedjson`** (`optimizer/core/embedjsonoptimizer`, first): JSON values with record arrays
   inside log/test/build/task/text blocks (`jsonx.Spans`) are crushed in place by jsonx; the text
   around them stays byte-exact.
2. *Family engines.*
3. **`dense`** (`optimizer/core/denseoptimizer`): long space-free lines (minified code, base64,
   data URIs) keep head/tail bytes around `…[N bytes dense]…`, once a block holds `min_total`
   such bytes. JSON lines and fs/code read from disk (file attr, `detect.ReadsFile`) are untouched.
4. **`paths`** (`optimizer/core/pathsoptimizer`): long path prefixes repeated in
   test/build/search/log/task blocks → `§1/file.go`, with a legend `[paths: §1=…]` on top of the
   first such block. Greedy by bytes saved, kept only when ≥ `min_saving` tokens, idempotent,
   never fs/data.
5. **`truncate`** (`optimizer/core/truncateoptimizer`): blocks still over `max_tokens` keep
   head/tail token budgets + `max_errors` error lines from the middle; long lines are cut.
   `skip`: fs/code, data, k8s, cloud.

### Shared libraries

- `compress/compose` sniffs a section and calls the test/build/pkg engine; used by `task` and
  docker builds.
- `compress/rows` samples records: errors, anomalies, change points, query matches, ends.
- `compress/probe` measures retention: error lines, artifacts (paths, URLs, hashes, UUIDs) and
  `key=number` pairs of an original that are still readable in the output (regrouping, path
  legends and short hashes count as kept).
  - `gascoigne eval` shows lost counts per case and pins them, with out tokens, in
    `testdata/eval/baseline.tsv`; any change fails until `-update`.
  - `gascoigne probe` runs it over ledger originals vs stored outputs (proxy folds skipped), per
    family + worst runs.

### JSON arrays (`compress/jsonx`)

Used by `optimizer/data/jsonoptimizer`, cloud, data and web.

1. **Small arrays are never sampled**: < `min_items`, < `min_tokens`, or ≤ `max_items`.
2. **Lossless first**: similar objects become a table (`_cols/_rows`), with constant columns in
   `_same` and shared prefixes/suffixes in `_affix`. Items with nested objects are also tried
   flattened to dotted columns (`jsonx.Flatten`); the smaller table wins. Kept when it saves
   ≥ `lossless_min_saving` and fits `lossless_max_tokens`.
3. **Else scored sampling** to ~`max_items`, in priority order:
   - errors, always: error fields, bad statuses, 4xx/5xx, keys or enum values few items have
   - by score: anomalies (> `anomaly_std` σ), change points (numeric jumps, enum flips), BM25
     matches of the `query` attr (`-attr query=...`, MCP `query` param)
   - head/tail shares, then an even sample
   - identical items collapse; gaps become `… N items omitted`

## MCP

**`gascoigne mcp`**, a stdio server with these tools:

| Tool | Does |
|---|---|
| `fetch` | web pages → markdown, API JSON compressed; http/https only, timeout, size cap. The cheap way to read web data |
| `compress` | any text, with optional `cmd`/`file` hints |
| `show {file, symbol?}` | one definition with doc comment and line numbers, or the outline without a symbol. Also `gascoigne show file[:symbol]`; `mcptool.Show` on `compress/symbols` (Go parser, Python indent, C-like braces) |
| `retrieve id` | the original; optional `lines`/`grep`/`query` read a part (not a failure). A full read marks the source as badly optimized |
| `stats` | savings (see Ledger → Reports) |

**`gascoigne mcp-proxy -- <server cmd>`** mirrors a downstream server's tools, optimizes their text
results and adds `gascoigne_retrieve`. Non-text content passes untouched.
- `compact_tools` drops redundant schema keys.
- `max_tools` > 0 (off by default) hides more tools than that behind `search_tools` (BM25 over
  names and descriptions, top 5 with schemas) and `call_tool`, for agents without native tool
  search.

MCP pipelines have no exporters: stdout is the protocol channel.

## LLM API proxy (`receiver/httpproxyreceiver`, `cmd/gascoigne/llmproxy.go`)

### Serving

- Mounted in the background server under `/v1/`: one process, one port, base URL
  `http://127.0.0.1:7878`.
- The receiver is an `http.Handler` (`Run` is a no-op), built from the user's config
  (`receivers.httpproxy`, pipeline `proxy` via `optimizerService`). Payloads run one at a time
  under the warm services' lock.
- On a new build the server waits until no proxied request is in flight (`waitIdle`, ≤ 10 min)
  before restarting.

### APIs

| API | Matched by | Notes |
|---|---|---|
| Anthropic `/v1/messages*` | path, or `anthropic-version`/`x-api-key` headers | |
| OpenAI `/chat/completions`, `/responses` | path | |
| Gemini `/v1beta/models/<m>:generateContent\|:streamGenerateContent` | path (also mounted at `/v1beta/`), or `x-goog-api-key` | `functionResponse.response.output\|content` answers the call with its id, else the oldest open call of its name; model from the path |

- Other paths and every header pass through. `anthropic-beta` carries the OAuth capability, so a
  claude.ai login keeps working.
- Responses stream through (`FlushInterval -1`).
- Any parse problem → the body is forwarded unchanged. `X-Gascoigne-Raw: 1` → untouched.

### Tool results

- One payload per tool result, with attrs from the matching call (`attrsOf`):
  - built-in tools are described as the shell command that prints the same thing: Read →
    `cat <path>` (or `sed -n <offset>,$p` for slices), Grep → `rg -n|-l|-c`, Glob/list → `find`
  - `query`/`prompt` → query attr; url; tool; model from the body; agent from the User-Agent
- `skip` tools, `is_error` results and hinted output stay untouched.
- Read's `N\t` prefixes are split off, the text is optimized like `cat`, and kept lines get their
  original numbers back (`renumber`).
- **Prompt cache**: results are cached by content hash (LRU `cache_entries`), so every later
  request sends the same bytes. Optimizers are deterministic, so a restart recomputes them.

### Fold (`fold.go`, per request)

After shortening, runs of ≥ `fold_min_lines` lines / `fold_min_chars` bytes that an earlier
result of the same request shows become one pointer: `[↑ L13-18 = L10-15 of earlier Read a.go]`.
Read lines match by text, with a uniform line-number shift.
- Recomputed for each request: a result depends only on earlier ones, so it is prefix-stable and
  leaves no dangling pointers when clients clear old results.
- The original is saved via `SaveRaw` (content-addressed id) + an event (route `fold`); savings
  are memoized. Needs `ledger` (looked up in `Start`).

### Other request parts

- **Tool schemas** (`tools.go`, `compact_tools`, `compress/schema`): titles restating names and
  `$schema`/`$id` are dropped in every API's shape; one event per distinct tool set (route `tools`).
- **Instructions** (`prompts.go`): system prompts, `<system-reminder>`s in user text, chat/responses
  system/developer messages and `instructions`, Gemini `systemInstruction`. Each distinct block
  records one unchanged event (route `prompt`, command `system prompt`/`CLAUDE.md`/`skills`/
  `reminder`), so `gain -by command` shows what instruction text costs.
  - The text is kept as the run's original (`gascoigne raw <id>`) and pruned with `raw_ttl` like
    other originals.
  - The dashboard shows only its size: `/api/raw|out` answer 403 for `prompt:` keys.
- **Skills** (`skills.go`): invoked skill bodies (user text starting
  `Base directory for this skill:`) are recorded once per body with their text kept (route
  `skill`, command `skill <name>`).
  - With `skill_code_lines`, longer code blocks keep their head plus
    `… N more lines: gascoigne raw <id> -lines a-b`. HTML comments go.
  - Measured: a 30k-token bundled skill is prose and tables (−0.3%).
- **Images** (`images.go`, `max_image_pixels`, 0 = off): base64 PNG/JPEG blocks in tool results
  above the cap are box-downscaled (deterministic, cached by hash). The original is saved (`raw`)
  and the event counted in image tokens (pixels/750, route `image`).

### Usage metering (`usage.go`)

- 2xx response bodies pass through a meter: SSE `data:` lines mentioning usage are decoded, JSON
  bodies ≤ 8 MB at the end.
- Per API reader, fields are max-merged across events; cached input is split out (OpenAI and
  Gemini count it in input).
- On close, one `component.Usage` goes to the ledger's optional `component.UsageRecorder`
  (table `usage`, no session).
- `gain` section "api usage", priced by `cost.Lookup`; JSON `api_usage`; `reset` filters apply.

### Registration with agents

The server is started first (`serverUp`). Then:

| Agent | Proxied when | How |
|---|---|---|
| claude (`claudeProxyVar`) | `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`, or `GASCOIGNE_PROXY=on`. A claude.ai login stays direct (terms of use) | `ANTHROPIC_BASE_URL` |
| claude on Bedrock | `AWS_BEARER_TOKEN_BEDROCK` | `ANTHROPIC_BEDROCK_BASE_URL=…/bedrock/<region>`; route `bedrock.go`, region validated, upstream template `bedrock`. SigV4 stays direct: it signs the body |
| codex | `OPENAI_API_KEY` | a `model_providers.gascoigne` (`wire_api=responses`) |
| opencode | each provider only with its API key | provider `baseURL`s |
| gemini | `GEMINI_API_KEY`. A Google login stays direct | `GOOGLE_GEMINI_BASE_URL` |

- Never on Vertex or Foundry, nor over a user's own base URL.
- `install -proxy` refuses in the same cases. `install -proxy claude` writes the env for good
  (SessionStart starts the server); unwrap removes it.

### Replay (`cmd/gascoigne/replay.go`)

`gascoigne replay` rebuilds Claude Code transcripts (`~/.claude/projects`, newest first) into one
request per model turn after tool results:
- lines of one role are merged, sidechains skipped, compaction restarts the conversation
- requests go through `llmProxy` against an in-process upstream with a throwaway ledger
- reports: tokens per tool; message input cost as sent and as forwarded (`cost` prices: the
  prefix the previous request sent identically is read from the cache, the rest written); how
  many re-sent messages the proxy forwarded byte-identical (canonical JSON)

## Source files (`optimizer/fs/catoptimizer`)

Agents edit by exact text, so source is shrunk only by dropping whole lines. Kept lines are
byte-exact: indentation and trailing comments are untouched, so Python stays valid as far as shown.

**Gap markers.** Each gap is one marker in the file's comment syntax that names the original
lines: `// … comments L12-30`, `# … docstring L3-9`, `// … L50-89` for a body (`N lines` instead
of numbers for `tail`/`sed`). An agent edits within kept runs and reads a gap back by range
(`sed -n 12,30p f`: small, passes untouched) or the whole file (`cat_mode=full`, `gascoigne raw`).

**Modes** (chosen by size):
- **clean** (800–4000 tokens): comment runs go.
  - A run right above code keeps its first line with words (the doc summary). So does a run one
    blank line above code at its indent, unless it comes before any code.
  - Python docstrings after `def`/`class` keep their summary line.
  - Runs cheaper than a marker stay, as do directives (`//go:`, `# type:`, `noqa`, shebang,
    coding, eslint/ts pragmas).
  - Lines inside multi-line strings (backticks, triple quotes, heredocs) are never taken for
    comments.
- **outline** (≥ 4000 tokens): clean, plus function bodies elided; signatures and closing braces
  kept as they are.

**Scope.**
- Language-agnostic: comment syntax per extension (`//` + `/* */`, `#`, `--`, `;`, `%`), else by
  family (`detect.Lang`). Unknown languages are left alone.
- Only `cat`/`head`/`tail` output and MCP `compress` are touched, never an agent's own
  file-read tool.

## Agents (`cmd/gascoigne/agents.go`, `hook.go`)

### Commands

| Command | Does |
|---|---|
| `wrap <agent> [args]` | starts the agent with gascoigne registered for that run only (no config files touched), `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` set |
| `install [-g] [agent...]` | writes the registering config into project files (`-g`: user files). Existing gascoigne entries replaced, JSON merged, backup kept (`install.go`) |
| `wrap -print <agent>` | prints that config |
| `unwrap [agent...]` | (default: all) removes exactly those entries from user and project configs: claude settings + `.mcp.json` + `claude mcp remove`, cursor hooks/mcp JSON, codex `config.toml` tables, pi extension. Other keys keep their order; the original is saved as `<file>.gascoigne-bak`; unparsable files are reported, not rewritten |
| `uninstall [-y]` | (`uninstall.go`) unwrap all + stop the server + delete gascoigne's config/data/cache/state dirs, an outside ledger's files (never its dir) and the binary; asks first |

### Per agent

| Agent | Per run | How shell output is optimized |
|---|---|---|
| claude | `--mcp-config` + `--settings`. Denies Grep/Glob (searches run through Bash) and WebFetch (pages go through gascoigne fetch); Read stays (Edit needs it); WebSearch stays | PreToolUse rewrites the command to `gascoigne exec -- <cmd>`. No permission decision: rules see the wrapped command |
| codex | `-c mcp_servers…` + `-c hooks.PostToolUse…` | PostToolUse runs the output through pipeline `hook` and replaces the result (`decision: block`); codex rewrites a command only by approving it. Trust the hook once with `/hooks` |
| pi | `--extension` (written to the user cache dir) | `tool_call` rewrites the bash command; no MCP |
| gemini | env `GEMINI_CLI_SYSTEM_SETTINGS_PATH` → run settings in the cache dir (real system settings merged in, user/project stdio servers proxied) | BeforeTool `hookSpecificOutput.tool_input` rewrites `run_shell_command` (no decision: approval stays); SessionStart adds the guide (`gemini.go`) |
| opencode | env `OPENCODE_CONFIG_DIR` (plugin in the cache dir) + `OPENCODE_CONFIG_CONTENT` (`mcp`, proxied servers, `instructions` guide unless the user has some) | plugin `tool.execute.before` rewrites `output.args.command` via `gascoigne hook opencode` (`opencode.go`) |
| cursor | — (config files only: `-print`) | preToolUse `updated_input`, `permission: allow` (shell approval stays with `beforeShellExecution`) |

### Usage guide (`guide.go`)

claude: a SessionStart hook (`additionalContext`). codex: `-c developer_instructions` per run, or
an `AGENTS.md` marked block. cursor: a project rule. pi: an `AGENTS.md` block. Unwrap removes them.

`guide(rewrites, mcp)` says only what the agent has: rewrite advice where commands are rewritten
(not codex), the fetch/compress/show/retrieve tools where gascoigne's MCP server is registered
(not pi; it gets the `gascoigne show` / `gascoigne raw` CLI forms). Codex and pi share the project
`AGENTS.md` block: the one installed last stays.

Level: `$GASCOIGNE_GUIDE`, else `guide` in `prefs.json` (state dir, `prefs.go`; set with
`install -guide`), else `full`. `short` keeps the path-id and hint lines; `off` injects nothing
and the next `install` removes the AGENTS.md block and cursor rule. Unknown values mean full.
The file is not `config.yaml`: a key there needs a change in `internal/config`.

### Other MCP servers (`proxy.go`)

Stdio servers start through `gascoigne mcp-proxy -- <cmd>`; remote servers stay direct.
- `wrap` passes same-named copies: claude `--mcp-config` beats configured scopes for the run
  (approved/enabled servers only); codex uses `-c` overrides.
- `install` rewrites `.mcp.json`, `~/.claude.json`, codex tables and cursor `mcp.json`.
- `unwrap` restores the original commands from the kept args.

### Hooks

- Hooks call `gascoigne hook <agent>`, which never fails the tool call: bad input → no answer →
  call unchanged.
- **After-the-fact replacement** (`posthook.go`), for results the command rewrite cannot reach:
  - claude PostToolUse `updatedToolOutput` in the tool's shape (Bash stdout, Grep content mode, `mcp__*`)
  - codex PostToolUse `decision: block` + `reason` (`^Bash$|^mcp__.*`)
  - gemini AfterTool `decision: deny` + `reason` (shell, `mcp_*`)
  - skipped: hinted output (`hintRe`) and gascoigne's own MCP tools. Read is never replaced
    (Claude numbers its lines).
- **Deny rules**: `install claude` adds the same deny rules as wrap (Grep, Glob, WebFetch) and
  records them in `$XDG_STATE_HOME/gascoigne/denies.json`; unwrap removes only recorded ones
  (`denies.go`).
- **Identity**: rewritten commands carry `-agent <name>` and `-session <hook session_id>` when
  `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` are unset (installed hooks). `wrap` sets the env, so its
  commands stay short (`execPrefix` in `hook.go`).

### Command rewriting rules (`hook.go`)

- **Wrapped**: every command of a `&&`/`;` list, except builtins, quiet, interactive and REPL
  commands (`passthrough`; unknown programs are detected by content).
- **Kept pipes**: trailing `| head` / `| tail` stages (they then cut optimized text). Before
  `head`, exec gets `-attr hint=top`: the stdout exporter prints the hint first so the cut keeps
  it. Other pipes stay as they are, since other filters would search the optimized text.
- **Left as they are**:
  - lines with loops, conditionals or `{ }` groups
  - redirects (except `2>&1`), subshells, `||`, background runs
  - commands that run gascoigne itself
  - commands that run until stopped (`-f`/`--follow`, `--watch`, `dev`, `serve`, `start`, `up`,
    plain `vite`), since exec shows output only when the command exits
- **Watch mode exception**: build tools in watch mode (`watched`) run as `exec -watch`, which
  processes each cycle when `watch.End` sees its result line; background runs included.

## Full-payload guarantee

Lives in core (`service/pipeline.go`), not in optimizers:

1. Before optimizers run, core snapshots the payload.
2. Output saving fewer than `minSaving` (20) tokens, the cost of the retrieve hint, → the original
   is kept, outcome unchanged. If the output differs, the original goes to the ledger first,
   `raw_id` is set, and stdout gets the hint
   `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. Ledger save fails → the original is returned. Optimizer errors → the original is returned and
   a failure recorded.
4. Up-front opt-out: the `bypass` filter (`-raw`, `GASCOIGNE_RAW=1`, attr `raw=true`).
5. Retrieval: `gascoigne raw <id>`, MCP `retrieve` / `gascoigne_retrieve` (later: HTTP proxy
   `GET /raw/<id>`).

## Ledger (gorm + sqlite, `extension/ledger`)

### Storage

- Pure-Go driver (`gormlite`), WAL mode, busy timeout 5 s.
- Gorm models in `models.go`; `AutoMigrate` on Start adds new tables, columns and indexes.
  Renames, drops or type changes need an explicit migration step: ask first.

| Table | Holds |
|---|---|
| `events` | every trip: route, tokens, outcome, `session`, `command`, `reason`, bypassing `filter` |
| `steps` | per event, each optimizer that changed tokens: optimizer, family, tokens before/after |
| `raw` | originals, gzip-compressed (plain JSON rows from before still read); TTL-pruned. `gascoigne flush [-older]` / `POST /api/flush` delete and VACUUM |
| `failures` | per-key failure count |
| `usage` | per proxied API request: agent, model, input, cache read/write and output tokens as the provider billed them |

**Raw ids** are sha256 prefixes of the original's text, attrs aside: the same output from two
sessions is stored once. 8 hex digits, unless that prefix holds another text (then 12, 16, 64):
~7 fewer tokens in every hint, still content-addressed and unique.

**Source keys** over 200 bytes (whole command lines, heredocs) are stored as a 160-byte prefix +
`…#<hash>` (`keys.go` `storedKey`, applied by every method). Ledgers from before were converted
once (`PRAGMA user_version` 1) and vacuumed: 22 MB → 6 MB on a real one, mostly the key index.

### Identity and sessions

- `-session ID`, `-agent NAME`, `-model NAME` on any command (env `GASCOIGNE_SESSION`,
  `GASCOIGNE_AGENT`, `GASCOIGNE_MODEL`) tag events.
- MCP: agent defaults to the client's name (clientInfo); model only when given. Session: explicit
  `-session`, else the MCP transport session id, else one id per server run.
- Hooks record the model from the tail of `transcript_path` (`transcriptModel`, `-model` in the
  rewrite).
- `events.command` is normalized via `detect.Command`, else MCP tool + URL host.

### Reports: `gascoigne gain`

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

### Cost (`cost/`, `mcptool.BuildCosts`, docs/cost.md)

saved tokens × tokenizer factor × (first read + rereads), per agent and model:
- tokenizer factor: claude 1.58 (measured from transcripts), legacy 1.25, gemini 0.79, o200k 1
- first read: cache write or input price; then `rereads` (measured median 110 later requests in
  context) × cache read price; 0.8% misses at write price
- runs without a model use `cost.AgentModels` (marked assumed)
- prices are the providers' official list prices (`cost.PriceSources`, `cost.PricesChecked`);
  `Costs.Pricing` carries that basis into the overview JSON, `estimates` and the dashboard

### Skip reasons

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

### Failure history

Signals → `MarkFailure(key, reason)`: optimizer error (`error`), agent fetched the raw original
(`raw_requested`), `gascoigne fail` (`manual`). The `history` filter bypasses a key once
`count >= max_failures`; `gascoigne forget <key>` resets it.

### Background server (`cmd/gascoigne/server.go`)

**Lifecycle**
- One per user on `$GASCOIGNE_SERVER_ADDR` (default `127.0.0.1:7878`; the port is the lock).
- Started detached (setsid, log in the user cache dir) when `/api/health` does not answer, by
  commands with `common.server` set (`exec`, `serve` for mcp/mcp-proxy, the codex hook's
  `optimize`) and by `wrap`.
- Health reports pid, build (hash of binary path + size + mtime), binary mtime and ledger path.
  An older build is replaced (stop + spawn), a newer one kept, another ledger left alone.
- The server stats its binary every 2 s. A new build seen twice in a row → shut down, close the
  ledger, spawn the new binary (`watchBinary`).
- **Idle exit** (`idle.go`): requests in flight and the last one's end are tracked (`/api/health`
  excluded; an open stream counts). After `-idle`/`GASCOIGNE_SERVER_IDLE` (30m, 0 never) the
  server shuts down. Commands start it again (`ensureServer`); agents' UserPromptSubmit hooks
  (claude, codex; opencode `chat.message`) bring it back before the next API request reaches the
  proxy.
- Commands: `server run|start|stop|status`; `dashboard` = `server start -open` (opens a browser
  unless there is no display; `-open=false`); `GASCOIGNE_SERVER=off` disables it.

**Server mode** (`remote.go`): `exec`, `run` and the codex hook (`common.useServer`) build only
their pipeline's receivers and exporters (`service.Options.Remote`) and POST the payload to
`/api/optimize`.
- There a warm service per config hash (defaults + user file + overrides; receivers and exporters
  stripped; ≤ 8 kept; one payload at a time) runs `Pipeline.Optimize`.
- Same build and ledger path required (409 otherwise). Refused, 409 or a broken server → the full
  pipeline is built locally for that payload (which also starts or replaces the server). Pipeline
  errors (422) are returned.
- Runs with other `GASCOIGNE_*` env (raw, cat mode, ledger) stay local. A spawned server drops the
  spawner's `GASCOIGNE_*` env except ledger/config/addr.
- Package-level patterns use `lazyre` (`TestPatternsCompile` runs `lazyre.Check`), so a served
  run never compiles them.

**Reset**: `gascoigne reset` / `POST /api/reset` → `ledger.Reset`: events + steps by filters
(flags only, never env); `-all` also raw and failures, with no filters.

### Dashboard (`dashboard.go`)

One embedded `dashboard.html`, no build step, serving the ledger live.
- Layout: version in the header, the logo moon's phase = savings rate; tables in equal-height
  boxes, rows rendered as they scroll; deleting data (reset, clear originals) asks twice.
- Content: totals, days, outcomes, model families and models (estimated savings in the model's
  tokens and dollars from `overview.cost`; `cost.Model.Family` = claude/gpt/gemini/other, summed
  in the page), commands, optimizers, dialects, skip/fail/unchanged reasons,
  failing sources (forget button), and a feed of runs over server-sent events (`/api/stream`,
  polls the ledger since other processes write it; `ledger.Recent`).
- Run details compare original and optimized side by side (`/api/raw/{id}`, `/api/out/{id}`; the
  optimized text is `Event.Output`, kept gzip in `raw.output`), with `#run-ID` deep links. A run
  opens its steps and original; viewing an original there does not mark the source as failing.
- Security: loopback Host names only (DNS rebinding); cross-origin writes refused.

## Config

### Files

- `configs/default.yaml` (embedded in the binary) holds every default.
- The first config file found is laid over it: `-config` > `$GASCOIGNE_CONFIG` >
  `$XDG_CONFIG_HOME/gascoigne/config.yaml` (`cmd/gascoigne/userconfig.go` `overlay`: maps merge,
  values and lists replace, empty keys keep the default).
- With neither flag nor env, the first run writes `configs/user.yaml` there: every setting of
  every component commented out (`##` docs, `#` settings).
- Upgrades: a later version refreshes a file that sets nothing. `config -upgrade`
  (`configupgrade.go`) rebuilds one with settings on the new template (unknown ones dropped and
  named, old file `.bak`). An unknown setting fails up front, naming the upgrade.
- Strict decoding: unknown fields fail. Tests keep the template honest: uncommented, it equals the
  defaults, and every `Config` field of every used component appears in it (a new field fails
  until documented).
- Pipelines may override `router` (proxy and CLI route differently). Optimizers configured in a
  pipeline's router require a ledger extension.

### Per-run overrides (`internal/config/override.go`)

So each agent can tune optimization without its own config file: flags on any command, or env set
once in the agent's environment. Flags win over env.

| Flag | Env | Effect |
|---|---|---|
| `-set path=yaml` (repeatable) | `GASCOIGNE_SET` (`;`-separated, applied before flags) | assign any config value: `-set optimizers.grep.max_per_file=50`, `-set router.default=[go,git]` |
| `-only a,b` | `GASCOIGNE_ONLY` | keep only these optimizers in every chain |
| `-disable a,b` | `GASCOIGNE_DISABLE` | drop these optimizers from every chain |

`gascoigne config [flags]` prints the effective config. Per payload, `-attr k=v` still sets attrs
(e.g. `cat_mode=full`).

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
