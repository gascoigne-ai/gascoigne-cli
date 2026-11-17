status: todo
kind: chore
commit: refactor(cmd): full parameter names in cmd-p-r
scope: cmd/gascoigne/[p-r]*
depends_on: 014-param-name-check

# Full parameter names in cmd-p-r

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes.
