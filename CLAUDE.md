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
- CLI reference (exec, mcp, mcp-proxy, wrap, install, gain, reset, tokens, prompts, probe, replay, flush, tuning flags): `bin/gascoigne help [cmd]`, [docs/cli.md](docs/cli.md)

## Harness
Shared with the other gascoigne repos: submodule `.harness/common`.
@.harness/common/rules.md
@.harness/common/commit-style.md
@.harness/common/code-style.md
@.harness/rules.md
@.harness/code-style.md
@.harness/recommendations.md

UI (dashboard and its pages): read `.harness/visual-style.md` (not loaded) before changing one, and look at the result with
`.harness/scripts/shot.sh`.

Why things are as they are: `.harness/decisions.md` (not loaded). Open ideas, measured results, dropped ideas:
`.harness/backlog.md` (not loaded; read before proposing or retrying an optimization).
Architecture reference: `.harness/architecture.md` (index, layout; loaded on demand) links one file per area in
`.harness/architecture/` (each ~15-140 lines). Read the file for the area you touch before changing it:
flow, detection, families, mcp, llm-proxy, source-files, agents, hooks, full-payload, ledger, server, reports,
config, adding-component. The index table says which files each one covers.

Changing behavior a file describes: update that file in the same commit.

Layout: public contracts `component/` `payload/` `tokens/`; components in `receiver/` `filter/` `optimizer/` (flat: `optimizer/<name>optimizer`) `exporter/` `extension/` (may import `internal/` libs, never the engine); engine in `internal/{config,router,service,registry}`; libs in `internal/` (`detect` `compress` `mcptool` `cost` `prose` `mcpauth` `exact` `version` `lazyre`: package-level regexps `lazyre.New`, never `regexp.MustCompile`; `pcomb`: line matchers and named rule tables with examples, preferred over a new long regexp alternation).

Skills: `verify`, `add-component`, `eval`, `release` (version, changelog, tag, push, website page). Task queue: `.harness/tasks/`, loop: `.harness/scripts/loop.sh`.
