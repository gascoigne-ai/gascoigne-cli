status: todo
kind: chore
commit: refactor(mcp): full parameter names in detect-mcp
scope: internal/detect/**, internal/mcptool/**, internal/mcpauth/**
depends_on: 014-param-name-check

# Full parameter names in detect-mcp

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes (the task touches code, so the check runs).
