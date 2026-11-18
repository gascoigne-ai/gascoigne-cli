status: todo
kind: feature
commit: feat(gain): filter and group by pipeline
scope: cmd/gascoigne/gainview.go, cmd/gascoigne/render.go, extension/ledger/**, .harness/architecture/ledger.md

# `gain -pipeline` filter, `-by pipeline`, detected type column

Real-ledger reads mix `proxy`, `cli` and `eval` traffic with live `exec`/`hook`/`mcp` traffic. A review
misread four sinks because of this (`cd` compounds, tool definitions, one history-bypassed fetch, one
eval fixture repeated 318 times). `gain` today filters only by session, agent, model and since.

Add:
- `-pipeline NAME[,NAME]` filter and `-by pipeline` grouping, next to the existing `-by` modes.
- A `type` column on `events` (detected family/dialect, for example `fs/code`) so `-by type` works
  without parsing `reason`. Migrate old rows with an empty value; do not backfill.

Test first: ledger Stats tests for the filter and the new grouping; a migration test for the column.

Acceptance: `gascoigne gain -pipeline exec,hook -by command -since 7d` runs against a throwaway ledger
and sums only those pipelines. `make check` passes. Update the ledger architecture doc in the same commit.

## Model
events gains a `type` text column, default empty (additive migration, no backfill). Stats group/filter by pipeline and type.
