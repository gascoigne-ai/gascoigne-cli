# Rules

Hard rules for every agent and human change. Breaking one = change rejected.

## Dependencies
- Stdlib first. Approved third-party deps:
  - `go.yaml.in/yaml/v3` — config
  - `gorm.io/gorm` — ledger ORM
  - `github.com/ncruces/go-sqlite3/gormlite` — gorm sqlite dialect, pure Go (wasm), no cgo.
    Never swap in a cgo driver (`gorm.io/driver/sqlite`, mattn): builds must stay `CGO_ENABLED=0`.
  - `golang.org/x/net` — HTML parsing (`html`, `html/atom`) for web data
  - `github.com/modelcontextprotocol/go-sdk` — MCP server/client
- New dep only when the user explicitly asks. Never add one to "save time".

## Verification
- `make check` (gofmt, vet, tests) must pass before you call work done.
- New behavior ships with tests. Table-driven where there are several cases.
- Optimizer changes: run `make eval`; savings must not regress and `raw ok` must never be `NO`.
- Bugs get a failing test first, then the fix.

## Architecture
- Extend by adding a component (receiver, filter, optimizer, exporter, extension) + config.
  Never special-case a component inside core.
- Core = public contracts `component/`, `payload/`, `tokens/` + engine `internal/{config,router,service}`.
  Changing core needs user approval. Contract changes break every component — treat as API changes.
- Module boundary: component dirs (`receiver/`, `filter/`, `optimizer/`, `exporter/`, `extension/`)
  import only public packages, never `internal/` (enforced by `make imports`). Each component dir
  must be able to become its own Go module later. No imports between component packages.
- New optimizer = new package + registry line + route in `configs/default.yaml` + eval case.
- Never lose information. Optimized output must stay reversible through the ledger;
  bypass output stays byte-exact. Optimizers never write to the ledger themselves — core does.
- Optimizers are deterministic: same input, same output (prompt caching + eval depend on it).

## Hygiene
- No competitor or product names of other token tools in code, docs, tasks, or commits.
  Describe techniques generically ("command output filtering", "JSON compaction").
- No speculative abstractions, options or hooks "for later".
- Small focused commits; don't touch unrelated files; don't reformat untouched code.
- No secrets, real user data or home paths in fixtures.
