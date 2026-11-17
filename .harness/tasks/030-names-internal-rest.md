status: todo
kind: chore
commit: refactor(internal): full parameter names in internal-rest
scope: internal/[a-c]*/**, internal/[e-l]*/**, internal/mcpmock/**, internal/[n-z]*/**
depends_on: 014-param-name-check, 007-price-refresh-before-december

# Full parameter names in internal-rest

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes. Engine packages are included; approved by the user.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes (the task touches code, so the check runs).
