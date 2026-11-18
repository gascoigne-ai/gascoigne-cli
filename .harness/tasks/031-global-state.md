status: todo
kind: chore
commit: refactor: configuration through factories, not globals
scope: internal/detect/**,cmd/gascoigne/**,receiver/**,internal/mcptool/**
depends_on: 021-lib-dedupe

# Remove global mutable state

- `internal/detect/registry.go:36`, `rows.go:23`: `specs`/`rows` filled by `register` in `init` and re-sorted per call -> one table, sorted once via `sync.OnceValue`.
- `cmd`: `openPage` (`release.go:155`) injected; `commandList` (`help.go:53`) via `sync.OnceValue`.
- Receivers carry config through `context.Value` (`WithRemote`, `WithLazy`, `WithTransport`, `WithRules`, exec/hook `With`, `mcptool.go:151`) -> typed options on the receiver set in the factory. Many tests change here: keep behavior, update setup only.

Run `SUITES=mcp make eval`. Acceptance: `make check`.
