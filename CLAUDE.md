# gascoigne

Token optimizer for LLM agents: 16 family engines (test, build, pkg, task, vcs, search, fs, table,
data, log, doc, web, db, k8s, infra, cloud) with ~150 tools/formats as declarative dialects
(`family/dialect` types),
MCP server + proxy, LLM API proxy. Go 1.27, module `github.com/gascoigne-ai/gascoigne-cli`.
Flow: receiver → filter → router → optimizer → exporter; sqlite ledger keeps originals + gains + failures.

## Commands
- `make check` — gofmt check, vet, import boundary, tests (must pass before done)
- `make eval` — every eval suite, one summary: `fixtures` (savings and fact retention vs `baseline.tsv`), `ext`, `scale` (big generated outputs, `max_ms`), `live` (real commands in this repo: catches tool output drift), `ledger` (benchmarks); `SUITES="scale live" make eval` picks some, extra flags go to each run (`-update`, `-show rg`)
- `make eval-experimental` — opt-in features apart from the default eval (`testdata/eval-experimental/`; today the prose model, needs `gascoigne prose install`, see `docs/prose-model.md`; recorded to your ledger as agent `eval`, so the dashboard shows the runs); never part of `make check`
- `bin/gascoigne eval -record [-cases file]` — the same run, written to your ledger (agent `eval`, a session per run; each case runs twice, so the counts double) so the dashboard shows it; `reset -agent eval` removes it
- `.harness/scripts/ab.sh [-n]` — task success with vs without gascoigne on `testdata/ab` tasks (calls the API; `-n` free dry run)
- `make build` → `bin/gascoigne`; `bin/gascoigne components`; `bin/gascoigne version` (release `version.Number` + git hash from build info, `-dirty` when uncommitted; also in `server status`, MCP server info)
- `bin/gascoigne exec -- <cmd>`, `bin/gascoigne mcp`, `bin/gascoigne mcp-proxy -- <server>`, LLM API proxy in the background server under `/v1/` (base URL http://127.0.0.1:7878: shortens tool results in requests, Read with line numbers kept; `wrap` sets the agent base URL, `install -proxy claude` for good; Claude only with `ANTHROPIC_API_KEY`/`ANTHROPIC_AUTH_TOKEN`, a claude.ai login stays direct unless `GASCOIGNE_PROXY=on`; `GASCOIGNE_PROXY=off`)
- agents: `bin/gascoigne wrap claude|codex|gemini|opencode|pi [args]` (per run), `install [-g] [agent...]` (for good: project, `-g` user configs; also cursor), `wrap -print <agent>` prints it, `unwrap [agent...]` removes it; hooks call `gascoigne hook <agent>`
- live view: background server auto-started by exec/hook/mcp/wrap → http://127.0.0.1:7878; `server status|stop|start|run`, `dashboard` opens it in a browser (`-open=false` only prints the URL), `GASCOIGNE_SERVER=off`; exec/run/codex hook optimize through it (`POST /api/optimize`, warm pipeline), else build the pipeline locally; it restarts itself when its binary changes and exits after 30m idle (`server run -idle`, `GASCOIGNE_SERVER_IDLE`, 0 never; health checks are not activity), restarted by the next command or agent prompt (UserPromptSubmit hook: claude, codex; opencode `chat.message`)
- `bin/gascoigne reset [-session|-agent|-since]` clears savings stats; `-all` also originals and failures
- `bin/gascoigne tokens [file]` counts text per tokenizer family and prices it, plus exact counts (`internal/exact/`: o200k in Go, vocabulary cached; Claude via `count_tokens` with config `tokenizers.anthropic_api_key` or `$ANTHROPIC_API_KEY`); `gain` has a cost section (`internal/cost/`, docs/cost.md); hooks record the model from the agent transcript
- `bin/gascoigne prompts [-agent claude|codex] [dir]` (experimental: config `experimental: [prompts]` or `GASCOIGNE_EXPERIMENTAL=prompts`, `all` for every one; commands with `experimental: true` in `help.go` are hidden and refused otherwise) lists the instruction text sent with every request (CLAUDE.md chain + imports, memory, skill/agent descriptions; AGENTS.md), prices it, flags large imports, duplicate lines, long descriptions; `prompts -terse 0.1 <files>` drops articles/fillers/hedges in place (`internal/compress/terse`, `.bak` kept)
- `bin/gascoigne probe [-since 7d]` checks recorded runs for error lines/artifacts their output lost
- `bin/gascoigne replay [-sessions N]` replays Claude Code transcripts through the LLM proxy: savings per tool, prompt-cache stability
- `bin/gascoigne flush [-older 7d]` deletes stored originals (gzip in `raw`) and vacuums
- savings: `bin/gascoigne gain [-session ID] [-json]` overview (essentials; `-full` all sections); `gain -by day|week|month|agent|model|session|command|optimizer|family|dialect|outcome|reason`, filters `-agent -model -session -since 7d`
- per agent/run tuning: `-set path=yaml`, `-only`, `-disable` (or `GASCOIGNE_SET/ONLY/DISABLE`); `bin/gascoigne config` shows the result

## Harness
Shared with the other gascoigne repos: submodule `.harness/common` (`git submodule update --init` in a new worktree).
@.harness/common/rules.md
@.harness/common/commit-style.md
@.harness/rules.md
@.harness/code-style.md
@.harness/recommendations.md

Why things are as they are: `.harness/decisions.md` (not loaded). Open ideas, measured results, dropped ideas:
`.harness/backlog.md` (not loaded; read before proposing or retrying an optimization).
Architecture reference: `.harness/architecture.md` (index, layout; loaded on demand) links one file per area in
`.harness/architecture/` (each ~15-140 lines). Read the file for the area you touch before changing it:
flow, detection, families, mcp, llm-proxy, source-files, agents, hooks, full-payload, ledger, server, reports,
config, adding-component. The index table says which files each one covers.

Changing behavior a file describes: update that file in the same commit.

Layout: public contracts `component/` `payload/` `tokens/`; components in `receiver/` `filter/` `optimizer/` (flat: `optimizer/<name>optimizer`) `exporter/` `extension/` (may import `internal/` libs, never the engine); engine in `internal/{config,router,service,registry}`; libs in `internal/` (`detect` `compress` `mcptool` `cost` `prose` `mcpauth` `exact` `version` `lazyre`: package-level regexps `lazyre.New`, never `regexp.MustCompile`; `pcomb`: line matchers and named rule tables with examples, preferred over a new long regexp alternation).

Skills: `verify`, `add-component`, `eval`, `release` (version, changelog, tag, push, website page). Task queue: `.harness/tasks/`, loop: `.harness/scripts/loop.sh`.
