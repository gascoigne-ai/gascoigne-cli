status: todo

# Compaction economics

## Goal
The longer a session runs without compaction, the more every further turn costs: each one re-reads
the whole context. A compaction costs one summary and one cache rewrite. Show where the break-even
is for real sessions, so users can pick a threshold.

## Scope
- From the `usage` table per session: context tokens per request over time. Simulate compacting at
  50, 70 and 90 percent of the window: cache reads saved on later turns against one summary
  (assume a summary size, state it) and one cache write. Print the best threshold in `gain`
  (section "compaction") with the assumptions in the `estimates` note.
- Report only. A status-line or prompt-time nudge is a separate task, after the numbers exist.
- Depends on task 050 for the per-session context series; reuse its query.

## Acceptance
- Fixture ledger tests with a growing session and a short one (no advice for short sessions).
- On this machine's ledger the report runs and its assumptions are printed; record a real result
  in `backlog.md`.
