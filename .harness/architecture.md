# Architecture

Gascoigne shrinks what LLM agents read (command output, tool results, API traffic) without
ever losing information. The design borrows from the OpenTelemetry Collector: typed pluggable
components built by factories, `type/name` ids, pipelines wired by YAML, an explicit registry.

Detail lives in one file per area under `architecture/`; read only the one for the area you touch.

## Layout

```
component/  payload/  tokens/        public contracts: what component authors import
receiver/  filter/  exporter/        components, one package each (OTel-contrib style)
extension/
optimizer/<name>optimizer            optimizers, flat; family is declared by the factory (see Families)
internal/config  router  service     engine: YAML, routing, pipeline execution
internal/registry                    built-in component list (the "distribution")
internal/detect  compress  mcptool   libs: content detection, text primitives, MCP tools
internal/lazyre                      package-level regexps compiled on first use (init stays ~1 ms)
internal/pcomb                       line matchers from combinators (Prefix, Word, Indented, Any, Re) and named rule tables with Yes/No examples
internal/version                     release number + git commit from build info (CLI, MCP, health)
internal/cost  prose  mcpauth        tokenizer families and prices, prose model runtime, MCP OAuth
internal/exact                       exact counts: o200k BPE in Go, Claude via count_tokens
cmd/gascoigne  configs/              CLI + embedded default config
```

- Receivers: stdin, exec, hook, mcp, mcp_proxy, httpproxy.
- Only `component/ payload/ tokens/` are public. Components import them plus the `internal/` libs, never the
  engine (`internal/{config,router,service,registry}`). `make imports` fails on either breach.
- `internal/exact/` downloads the o200k vocabulary once and pins its hash. BPE merging is O(n log n) per piece (heap), so long space-free pieces do not stall; HTTP requests time out after 30s.
- `internal/cost/` relates tokenizer families to `tokens.Estimate` by a factor.

## Reference

| File | Read when touching |
|---|---|
| [flow](architecture/flow.md) | pipeline order, `payload/` attrs |
| [detection](architecture/detection.md) | `internal/detect/`, `filter/detectfilter`, new dialects |
| [families](architecture/families.md) | any `optimizer/*`, `internal/compress/*` engines, `pcomb` rule tables, `embedjson`/`dense`/`paths`/`truncate` |
| [mcp](architecture/mcp.md) | `receiver/mcp*`, `internal/mcptool/`, `sdk.go` |
| [llm-proxy](architecture/llm-proxy.md) | `receiver/httpproxyreceiver`, `llmproxy.go`, `replay.go` |
| [source-files](architecture/source-files.md) | `optimizer/catoptimizer` |
| [agents](architecture/agents.md) | `agents.go`, `install.go`, `unwrap.go`, `guide.go`, `proxy.go` |
| [hooks](architecture/hooks.md) | `hook.go`, `posthook.go`, tool-result coverage, command rewriting, resume snapshot |
| [full-payload](architecture/full-payload.md) | `internal/service`, hints, raw ids |
| [ledger](architecture/ledger.md) | `extension/ledger`, `gain`, cost, lessons, skip reasons |
| [server](architecture/server.md) | `cmd/gascoigne/server.go` |
| [reports](architecture/reports.md) | `context.go`, `mcpreview.go`, `dashboard.go` and their pages |
| [config](architecture/config.md) | `configs/`, `internal/config`, overrides |
| [adding-component](architecture/adding-component.md) | new receiver/filter/optimizer/exporter/extension |

Changing behavior a file describes: update that file in the same commit.
