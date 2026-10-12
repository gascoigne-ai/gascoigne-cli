status: todo

# Guide line: batch independent tool calls

## Goal
Every model request re-reads the whole context (225M cache-read tokens in 30 days of real sessions).
One turn with N tool calls costs one context read instead of N. Agents rarely batch unprompted.
Ask for it in the session-start guide.

## Scope
- One short line in `guide.go` ("When calls do not depend on each other, issue them in one turn"),
  for every agent that has a guide. Bump `registrationVersion` and add a `registrationNotes` entry
  (`installs.go`), since the guide text changes.
- `ab.sh`: report requests per task (from the `usage` table) next to success and cost.
- Backlog: record the measurement under "Measured" (kept) or "Tried and dropped".

## Acceptance
- Guide golden tests updated; `make check` passes.
- `ab.sh` with and without the line on `testdata/ab` tasks, 3 repeats: success must stay 100%;
  keep the line only if requests per task or cost drops. Note that OpenRouter runs are noisy
  (see backlog, 2026-10-11/12 entries) and rerun cold-cache outliers.
- Same A/B can carry F1 ("report conclusions, never restate tool output") as a second arm.
