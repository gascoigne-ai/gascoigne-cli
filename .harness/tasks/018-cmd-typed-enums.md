status: todo
kind: chore
commit: refactor(cmd): typed toggle and finding level
scope: cmd/gascoigne/**

# Replace magic strings in cmd with types

- `finding.level` (`doctor.go:17`, compared in `doctor.go:33`, `upgrade.go:55`) -> `findingLevel` enum.
- 39 `"on"`/`"off"` literals (`setup_decisions.go`, `installs.go`, `release.go`, `mcpnudge.go`, `guide.go`, `install.go`) -> one `toggle` type like `mcpmode.go`.
- `== "off"` env switch repeated at `llmproxy.go:24,28`, `server.go:49`, `main.go:314,323`, `installs.go:124` -> `envOff(name)`.

Persisted prefs keep their on-disk strings. Refactor: tests unchanged and green. Acceptance: `make check`.
