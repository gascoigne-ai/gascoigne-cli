status: todo
kind: chore
commit: refactor(cmd): break up functions over 60 lines
scope: cmd/gascoigne/**, internal/mcptool/**, internal/pricing/**

# Long functions

From the audit (awk estimates; verify). Split by phase, keep names that say the phase:
- `eval.go` `cmdEval` 159, `runCase` 96; `mcpreview.go` `buildReview` 123; `pricing.go` `runPricingSet` 99;
  `ledger.go:133` 87; `replay.go` `cmdReplay` 73, `replayFile` 65; `hook.go` `cmdHook` 68;
  `server.go` `cmdServer` 64; `mcptool/costs.go` `BuildCosts` 79; `pricing.go` `Load` 65.
- `help.go:55` 242-line `init()` filling the package-level `commandList`: make it a composite literal
  (or `sync.OnceValue` if an init cycle blocks it).
- Nesting over 2: `receiver/httpproxyreceiver/formats.go`, `replay.go`, `continuity.go`.

Not in scope: `internal/service` (core, needs approval), `internal/cost` globals (backlog).

Acceptance: no touched function over 60 lines, tests unchanged and green, `make check` passes. Do one file
per commit step; skip a function when splitting makes it harder to read.
