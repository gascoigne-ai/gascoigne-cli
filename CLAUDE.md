# gascoigne

Token optimizer for LLM agents: 16 family engines (test, build, pkg, task, vcs, search, fs, table,
data, log, doc, web, db, k8s, infra, cloud) with ~110 tools/formats as declarative dialects
(`family/dialect` types),
MCP server + proxy, LLM API proxy. Go 1.27, module `github.com/gascoigne-ai/gascoigne-cli`.
Flow: receiver → filter → router → optimizer → exporter; sqlite ledger keeps originals + gains + failures.

## Commands
- `make check` — gofmt check, vet, import boundary, tests (must pass before done)
- `make eval` — token savings and fact retention on `testdata/eval` fixtures (vs `baseline.tsv`)
- `make eval-live` — the same checks on live commands in this repo (`testdata/eval/live.yaml`): catches tool output drift
- `.harness/scripts/ab.sh [-n]` — task success with vs without gascoigne on `testdata/ab` tasks (calls the API; `-n` free dry run)
- `make build` → `bin/gascoigne`; `bin/gascoigne components`; `bin/gascoigne version` (release `version.Number` + git hash from build info, `-dirty` when uncommitted; also in `server status`, MCP server info)
- `bin/gascoigne exec -- <cmd>`, `bin/gascoigne mcp`, `bin/gascoigne mcp-proxy -- <server>`, LLM API proxy in the background server under `/v1/` (base URL http://127.0.0.1:7878: shortens tool results in requests, Read with line numbers kept; `wrap` sets the agent base URL, `install -proxy claude` for good; Claude only with `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`, a claude.ai login stays direct unless `GASCOIGNE_PROXY=on`; `GASCOIGNE_PROXY=off`)
- agents: `bin/gascoigne wrap claude|codex|gemini|opencode|pi [args]` (per run), `install [-g] [agent...]` (for good: project, `-g` user configs; also cursor), `wrap -print <agent>` prints it, `unwrap [agent...]` removes it; hooks call `gascoigne hook <agent>`
- live view: background server auto-started by exec/hook/mcp/wrap → http://127.0.0.1:7878; `server status|stop|start|run`, `dashboard` opens it in a browser (`-open=false` only prints the URL), `GASCOIGNE_SERVER=off`; exec/run/codex hook optimize through it (`POST /api/optimize`, warm pipeline), else build the pipeline locally; it restarts itself when its binary changes and exits after 30m idle (`server run -idle`, `GASCOIGNE_SERVER_IDLE`, 0 never; health checks are not activity), restarted by the next command or agent prompt (UserPromptSubmit hook: claude, codex; opencode `chat.message`)
- `bin/gascoigne reset [-session|-agent|-since]` clears savings stats; `-all` also originals and failures
- `bin/gascoigne tokens [file]` counts text per tokenizer family and prices it, plus exact counts (`tokenizer/`: o200k in Go, vocabulary cached; Claude via `count_tokens` with config `tokenizers.anthropic_api_key` or `$ANTHROPIC_API_KEY`); `gain` has a cost section (`cost/`, docs/cost.md); hooks record the model from the agent transcript
- `bin/gascoigne prompts [-agent claude|codex] [dir]` (experimental: config `experimental: [prompts]` or `GASCOIGNE_EXPERIMENTAL=prompts`, `all` for every one; commands with `experimental: true` in `help.go` are hidden and refused otherwise) lists the instruction text sent with every request (CLAUDE.md chain + imports, memory, skill/agent descriptions; AGENTS.md), prices it, flags large imports, duplicate lines, long descriptions; `prompts -terse 0.1 <files>` drops articles/fillers/hedges in place (`compress/terse`, `.bak` kept)
- `bin/gascoigne probe [-since 7d]` checks recorded runs for error lines/artifacts their output lost
- `bin/gascoigne replay [-sessions N]` replays Claude Code transcripts through the LLM proxy: savings per tool, prompt-cache stability
- `bin/gascoigne flush [-older 7d]` deletes stored originals (gzip in `raw`) and vacuums
- savings: `bin/gascoigne gain [-session ID] [-json]` overview; `gain -by day|week|month|agent|model|session|command|optimizer|family|dialect|outcome|reason`, filters `-agent -model -session -since 7d`
- per agent/run tuning: `-set path=yaml`, `-only`, `-disable` (or `GASCOIGNE_SET/ONLY/DISABLE`); `bin/gascoigne config` shows the result

## Harness
@.harness/rules.md
@.harness/code-style.md
@.harness/recommendations.md

Architecture reference: `.harness/architecture.md` (not loaded; ~14k tokens). Read the section
for the area you touch before changing it (`sed -n '/^## Ledger/,/^## /p' .harness/architecture.md`):
| Section | Read when touching |
|---|---|
| Layout, Flow, Payload | package boundaries, pipeline order, `payload/` attrs |
| Content detection | `detect/`, `filter/detectfilter`, new dialects |
| Families | any `optimizer/<family>`, `compress/*` engines, `embedjson`/`dense`/`paths`/`truncate` |
| MCP | `receiver/mcp*`, `mcptool/` |
| LLM API proxy | `receiver/httpproxyreceiver`, `llmproxy.go`, `replay.go` |
| Source files | `optimizer/fs/catoptimizer` |
| Agents | `agents.go`, `hook.go`, `posthook.go`, `install.go`, `unwrap.go`, `guide.go` |
| Full-payload guarantee | `internal/service`, hints, raw ids |
| Ledger | `extension/ledger`, `gain`, `server.go`, `dashboard.go`, cost |
| Config | `configs/`, `internal/config`, overrides |
| Adding a component | new receiver/filter/optimizer/exporter/extension |

Changing behavior a section describes: update that section in the same commit.

Layout: public contracts `component/` `payload/` `tokens/`, libs `detect/` `compress/` `mcptool/` `cost/` `tokenizer/` `lazyre/` (package-level regexps: `lazyre.New`, never `regexp.MustCompile`); components in `receiver/` `filter/` `optimizer/` `exporter/` `extension/` (never import `internal/`); engine in `internal/`.

Skills: `verify`, `add-component`, `eval`. Task queue: `.harness/tasks/`, loop: `.harness/scripts/loop.sh`.
