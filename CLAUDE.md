# gascoigne

Token optimizer for LLM agents: 16 family engines (test, build, pkg, task, vcs, search, fs, table,
data, log, doc, web, db, k8s, infra, cloud) with ~110 tools/formats as declarative dialects
(`family/dialect` types),
MCP server + proxy, LLM API proxy (planned). Go 1.27, module `github.com/lukaszb/gascoigne`.
Flow: receiver → filter → router → optimizer → exporter; sqlite ledger keeps originals + gains + failures.

## Commands
- `make check` — gofmt check, vet, import boundary, tests (must pass before done)
- `make eval` — token savings on `testdata/eval` fixtures
- `make build` → `bin/gascoigne`; `bin/gascoigne components`
- `bin/gascoigne exec -- <cmd>`, `bin/gascoigne mcp`, `bin/gascoigne mcp-proxy -- <server>`
- agents: `bin/gascoigne wrap claude|codex|gemini|opencode|pi [args]` (per run), `install [-g] [agent...]` (for good: project, `-g` user configs; also cursor), `wrap -print <agent>` prints it, `unwrap [agent...]` removes it; hooks call `gascoigne hook <agent>`
- live view: background server auto-started by exec/hook/mcp/wrap → http://127.0.0.1:7878; `server status|stop|start|run`, `dashboard` prints the URL, `GASCOIGNE_SERVER=off`; exec/run/codex hook optimize through it (`POST /api/optimize`, warm pipeline), else build the pipeline locally; it restarts itself when its binary changes
- `bin/gascoigne reset [-session|-agent|-since]` clears savings stats; `-all` also originals and failures
- `bin/gascoigne tokens [file]` counts text per tokenizer family and prices it; `gain` has a cost section (`cost/`, docs/cost.md); hooks record the model from the agent transcript
- `bin/gascoigne flush [-older 7d]` deletes stored originals (gzip in `raw`) and vacuums
- savings: `bin/gascoigne gain [-session ID] [-json]` overview; `gain -by day|week|month|agent|model|session|command|optimizer|family|dialect|outcome|reason`, filters `-agent -model -session -since 7d`
- per agent/run tuning: `-set path=yaml`, `-only`, `-disable` (or `GASCOIGNE_SET/ONLY/DISABLE`); `bin/gascoigne config` shows the result

## Harness
@.harness/rules.md
@.harness/code-style.md
@.harness/architecture.md
@.harness/recommendations.md

Layout: public contracts `component/` `payload/` `tokens/`, libs `detect/` `compress/` `mcptool/` `cost/` `lazyre/` (package-level regexps: `lazyre.New`, never `regexp.MustCompile`); components in `receiver/` `filter/` `optimizer/` `exporter/` `extension/` (never import `internal/`); engine in `internal/`.

Skills: `verify`, `add-component`, `eval`. Task queue: `.harness/tasks/`, loop: `.harness/scripts/loop.sh`.
