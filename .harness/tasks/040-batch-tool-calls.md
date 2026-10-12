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
- Backlog: record the measurement under "Measured" (kept) or "Tried and dropped".

## Acceptance
- Guide golden tests updated; `make check` passes.
- No paid A/B. Measure on real use: requests per session from the proxy's `usage` rows (task 050)
  for the weeks before and after, noted in `backlog.md` as an observation, not a proof. Drop the line
  if the numbers do not move after two weeks.
- F1 ("report conclusions, never restate tool output") may ride in the same guide change.
