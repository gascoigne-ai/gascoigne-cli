status: todo

# Keep raw ids across compaction

## Goal
After compaction the agent forgets which outputs it already shortened and runs `go test`, `git log`
or a fetch again. Add the session's recent shortened commands and their raw ids to the resume
snapshot, so it retrieves instead of re-running.

## Scope
- `continuity.go` (experimental `continuity`): add the last N (default 8) shortened commands of the
  session from the ledger, newest first: normalized command, tokens before/after, raw id.
  Counted against the existing ~2 KB cap; the oldest go first.
- Only runs with a raw id and a hint; never output text, only the command key. The snapshot's
  secrets rule applies (`NAME_KEY=` and bearer values cut).
- No new hook registration.

## Acceptance
- Fixture transcript and ledger test: cap, determinism, order, secrets cut, session isolation.
- A/B that forces a compaction (the open backlog item): tokens added by the snapshot vs reruns
  avoided. Record the result in `backlog.md`.
