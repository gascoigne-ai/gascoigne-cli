status: todo
kind: chore
commit: refactor(cmd): login and run statuses are typed
scope: cmd/gascoigne/loginserver*.go, cmd/gascoigne/dashboard*.go, .harness/architecture/mcp.md

# Typed statuses for the login server and dashboard rows

The login server state is a string compared to `running`, `done`, `failed`, and dashboard rows carry outcomes as strings. Replace them with enums; JSON values stay the same.

## Model
`loginPhase`: running, done, failed. Transitions: running to done, running to failed; nothing leaves done or failed. Dashboard outcome uses `component.Outcome`. JSON values unchanged (persisted/wire compatibility).

## Acceptance
- A transition-table test and a JSON round-trip test first.
- `make check` passes (the task touches code, so the check runs).
