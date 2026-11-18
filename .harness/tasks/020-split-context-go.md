status: todo
kind: refactor
commit: refactor(cmd): split context.go by job
scope: cmd/gascoigne/context*.go, .harness/architecture/reports.md, .harness/architecture.md

# Split context.go (1052 lines)

It mixes four jobs. Split in the same package, no API change:
- `context_session.go`: transcript parsing. `readSession` (:256, 74 lines, holds 10 of the file's 11
  deep-nesting hits) becomes per-line-kind handlers.
- `context_cost.go`: `simulate`, `turnCost`, `effectiveModel`.
- `context_report.go`: `buildContext` (:643, 129 lines) becomes per-section builders.
- `context_print.go`: `printContext`, `printContextFlat`, `printCacheTTL`.

Steps: pin current output with a test over a real session fixture if absent, then move code, then break up
the long functions as a second step. Update file names in `reports.md` and the `architecture.md` table.

Acceptance: `make check` passes; no function over 60 lines or nesting over 2 in the new files; report
output byte-identical.
