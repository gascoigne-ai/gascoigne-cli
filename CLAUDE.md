# gascoigne

Token optimizer for LLM agents: command output filtering, context compression, MCP server,
LLM API proxy. Go 1.27, module `github.com/lukaszb/gascoigne`.
Flow: receiver → filter → router → optimizer → exporter; sqlite ledger keeps originals + gains + failures.

## Commands
- `make check` — gofmt check, vet, import boundary, tests (must pass before done)
- `make eval` — token savings on `testdata/eval` fixtures
- `make build` → `bin/gascoigne`; `bin/gascoigne components`

## Harness
@.harness/rules.md
@.harness/code-style.md
@.harness/architecture.md
@.harness/recommendations.md

Layout: public contracts `component/` `payload/` `tokens/`; components in `receiver/` `filter/` `optimizer/` `exporter/` `extension/` (never import `internal/`); engine in `internal/`.

Skills: `verify`, `add-component`, `eval`. Task queue: `.harness/tasks/`, loop: `.harness/scripts/loop.sh`.
