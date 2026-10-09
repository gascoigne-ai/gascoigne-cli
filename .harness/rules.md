# Rules

Hard rules for every agent and human change. Breaking one = change rejected.

## Dependencies
- Stdlib first. Approved third-party deps:
  - `go.yaml.in/yaml/v3` — config
  - `modernc.org/sqlite` — ledger (pure Go, no cgo)
- New dep only when the user explicitly asks. Never add one to "save time".

## Verification
- `make check` (gofmt, vet, tests) must pass before you call work done.
- New behavior ships with tests. Table-driven where there are several cases.
- Optimizer changes: run `make eval`; savings must not regress and `raw ok` must never be `NO`.
- Bugs get a failing test first, then the fix.

## Architecture
- Extend by adding a component (receiver, filter, optimizer, exporter, extension) + config.
  Never special-case a component inside core.
- Core = `internal/{component,config,router,service,payload}`. Changing core needs user approval.
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
