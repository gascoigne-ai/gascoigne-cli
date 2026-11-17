status: todo
kind: chore
commit: refactor(cmd): full parameter names in cmd-d-f
scope: cmd/gascoigne/[d-f]*
depends_on: 014-param-name-check

# Full parameter names in cmd-d-f

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes (the task touches code, so the check runs).
