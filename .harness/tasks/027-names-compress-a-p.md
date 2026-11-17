status: todo
kind: chore
commit: refactor(compress): full parameter names in compress-a-p
scope: internal/compress/*.go, internal/compress/[a-p]*/**
depends_on: 014-param-name-check

# Full parameter names in compress-a-p

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes (the task touches code, so the check runs).
