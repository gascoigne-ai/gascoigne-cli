status: todo

# Turns and context size in gain

## Goal
Every request re-reads the whole context, so the cost of a session is turns times context, not the
size of any one output. `gain` shows savings per tool result but not the number that drives the
bill. Show it, so the other token tasks can be judged by it.

## Scope
- From the `usage` table (per proxied request, with session): requests per session, mean and max
  context tokens per request (input plus cache read plus cache write), cache-read cost per turn.
- `gain -by session` gets the columns; overview gets one line: "N sessions, mean M turns, context
  grew from A to B". JSON fields and MCP `stats` follow `render.go` conventions.
- If `usage` has no session column, ask before adding one (AutoMigrate rules in architecture.md).

## Acceptance
- Fixture ledger test: counts, mean, max, empty window, sessions with one request.
- The numbers match `replay` on the same transcripts within rounding; say how rounding is done in
  the `estimates` note.
