# Architecture

gascoigne is built like the OpenTelemetry Collector: typed, pluggable components built by
factories, identified as `type` or `type/name`, wired into pipelines by YAML, listed in an
explicit registry.

## Flow

```
           ┌──────────────── pipeline (sync, request → response) ────────────────┐
input ─▶ receiver ─▶ filter* ─▶ router ─▶ optimizer chain ─▶ exporter* ─▶ output
                   (bypass, detect,          (each optimizer touches only
                    minsize, history)         blocks of its content types)
                        │ bypass                    ▲
                        └───────────────────────────┘ (skip optimizers)

core: snapshot original ─▶ ledger (raw + event + steps) ─▶ raw_id + hint
history filter ◀── ledger failures (source optimized badly → bypass next time)
```

1. A **receiver** turns input (stdin, a command's output, an agent hook event, an MCP call)
   into a `Payload` and hands it to the pipeline.
2. **Filters** run in order. Each returns `Continue` or `Bypass` and may tag attributes:
   `bypass` honors raw requests, `detect` types every block, `minsize` skips small payloads,
   `history` skips sources that optimized badly before.
3. The **router** picks an optimizer chain: the first matching route, else `default`.
4. **Optimizers** rewrite the payload in place. Each touches only blocks whose detected type
   it understands, so one chain serves every input.
5. The core saves the original to the ledger if the output changed, records the event, and
   sets the hint attributes.
6. **Exporters** write the result (stdout). MCP and hook pipelines have none: the result goes
   back to the caller.

## Components

| Kind | Contract | Role |
|---|---|---|
| receiver | `Run(ctx)`; gets `Settings.Pipeline` | turn input into payloads, return results to the caller |
| filter | `Filter(ctx, p) (Decision, error)` | continue or bypass; may tag attributes |
| optimizer | `Optimize(ctx, p) error` | rewrite the payload in place, deterministically |
| exporter | `Export(ctx, p) error` | write or forward the result |
| extension | lifecycle only | shared services; `ledger` implements `component.Ledger` |

Every component has `Start(ctx, Host)` and `Shutdown(ctx)` (`component.Nop` provides empty
ones). Start order: extensions, exporters, optimizers, filters, receivers; shutdown in reverse.
Components find shared services with `host.Extension(id)` in `Start`.

```
$ gascoigne components
kind       family  type
receiver   -       stdin, exec, hook, mcp, mcp_proxy
filter     -       bypass, minsize, detect, history
optimizer  test    test          optimizer  db      db
optimizer  build   build         optimizer  k8s     k8s
optimizer  vcs     git           optimizer  cloud   cloud
optimizer  search  grep          optimizer  infra   infra
optimizer  fs      ls, cat       optimizer  task    task
optimizer  data    json, data    optimizer  web     http, html
optimizer  log     log           optimizer  doc     doc
optimizer  pkg     pkg           optimizer  core    passthrough
optimizer  table   table
exporter   -       stdout
extension  -       ledger
```

## Payload

```go
payload.Payload{
    Blocks: []Block{{Kind, Text, Type}},
    Attrs:  map[string]string{},
}
```

- **Kind** is where a block came from: `cmd_output`, `tool_result`, `message`, `json`.
- **Type** is what it is, set by detection: `family/dialect`.
- **Attrs** carry context: `cmd`, `file`, `url`, `tool`, `mime`, `query`, `session`, `raw`,
  `raw_id`, `route`, `raw_tokens`, `out_tokens`, and `skip_reason` for components explaining
  why they left a payload alone.
- `Key()` is the stable source identity used for failure history: `file:` over `url:` over
  `cmd:` over `tool:`.

Optimizers change blocks through `p.Rewrite(fn, types…)`, which only visits blocks of the given
types.

## Content detection

`detect/` holds one declarative `Spec` per dialect, grouped by family in `detect/<family>.go`:

```go
Spec{
    Type:     GitLog,                       // "vcs/git-log"
    Cmds:     []string{"git log"},          // "program sub sub -flag", most specific wins
    Files:    []string{"*.diff"},           // base name globs
    MIME:     []string{"text/x-diff"},
    Sniff:    anyLine(`^commit [0-9a-f]{40}`),
    Priority: 41,                           // sniffing order: specific before generic
    Refines:  []string{"data/yaml"},        // generic types this one specializes
}
```

Evidence order: command → file/MIME → sniffing → `doc/text`. `CmdFunc` covers what patterns
cannot (flag values like `-o yaml`). `detect.Argv` unwraps wrappers and runners before
matching. `detect/matrix_test.go` requires a command example for every dialect with command
patterns and an eval fixture for every dialect with a sniffer.

## Families and shared libraries

One engine per family lives in `optimizer/<family>/<name>optimizer`. Logic that more than one
engine needs lives in public libraries under `compress/`:

| Library | Used for |
|---|---|
| `compress` | lines, ANSI stripping, `… N omitted` markers, head/tail, dedupe, severity |
| `compress/testreport` | test output rules (test family) |
| `compress/diag` | diagnostics grouping (build family) |
| `compress/pkgnoise` | package manager noise (pkg family) |
| `compress/compose` | sniff a section and hand it to the test/build/pkg engine (task, docker builds) |
| `compress/table` | parse and shrink tables (table, db, k8s, cloud, vcs, data) |
| `compress/rows` | sample records: errors, anomalies, change points, BM25 query matches, ends |
| `compress/jsonx` | the JSON crusher (data, cloud, web) |
| `compress/yamlx` | YAML prune rules (k8s, cloud) |
| `compress/htmlx` | HTML to markdown (web, MCP fetch) |
| `compress/watch` | watch-mode build cycles (build engine, `exec -watch`) |

Other public libraries: `detect` (content types), `mcptool` (MCP tools and reports), `cost`
(tokenizer families and prices), `lazyre` (package-level patterns compiled on first use, so a
short CLI run never pays for them), `tokens` (the BPE-style token estimate).

## Layout

```
component/  payload/  tokens/        public contracts: what component authors import
detect/  compress/  mcptool/         public libraries
cost/  lazyre/
receiver/  filter/  exporter/        components, one package each
extension/
optimizer/<family>/<name>optimizer   optimizers grouped by family
internal/config  router  service     engine: YAML, routing, pipeline execution
internal/registry                    the built-in component list
cmd/gascoigne                        CLI, agent integration, background server (dashboard, LLM proxy)
configs/default.yaml                 embedded default config
```

Component packages import only public packages, never `internal/`, and never each other, so
each could become its own Go module. `make imports` enforces it.

## Design rules

- **Never lose information.** Optimized output is reversible through the ledger; bypassed
  output is byte-exact. Optimizers never write to the ledger themselves; the core does.
- **Deterministic.** Same input, same output. No timestamps or random ids in optimized text,
  so prompt caches stay warm and evals are stable.
- **Extend by adding components**, never by special-casing one inside the core.
- **Worth the hint.** Output that saves fewer tokens than its retrieve hint costs (20) is
  left as is.
- **Errors fall back to the original.** An optimizer that fails returns an error; the core
  returns the original and records the failure.
- **Explain skips.** A filter that bypasses, or an optimizer that leaves its blocks alone on
  purpose, sets `skip_reason` with a factual reason (numbers, thresholds), which `gain` shows.

## Background server

One per user on `127.0.0.1:7878`, started detached by the first command that needs it:

- **Dashboard** — the ledger live, run details with original and optimized side by side.
- **Warm pipelines** — `exec`, `run` and the codex hook hand their payloads to
  `POST /api/optimize` instead of building the pipeline themselves; with no server (or
  `GASCOIGNE_SERVER=off`) they build it locally.
- **LLM API proxy** — `/v1/` forwards agents' API requests, shortening tool results on the way
  (`receiver/httpproxyreceiver`, an `http.Handler` the server mounts). Each result is transformed
  once and cached by content hash, so the provider's prompt cache keeps hitting.
- **Lifecycle** — replaces itself when its binary changes (after in-flight proxied requests
  finish), exits after 30 minutes idle; commands and agents' prompt hooks start it again.
