status: todo
kind: chore
commit: refactor(optimizer): full parameter names in opt-g-k
scope: optimizer/[g-k]*/**
depends_on: 014-param-name-check

# Full parameter names in opt-g-k

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes.
