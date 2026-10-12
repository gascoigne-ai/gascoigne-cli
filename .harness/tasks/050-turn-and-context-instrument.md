status: todo

# Turns and context size in gain

## Goal
Every request re-reads the whole context, so the cost of a session is turns times context, not the
size of any one output. `gain` shows savings per tool result but not the number that drives the
bill. Show it, so the other token tasks can be judged by it.

## Scope
- From the `usage` table (per proxied request). It has no session id (the LLM proxy cannot see one;
  docs/ledger.md), so group by day, agent and model: requests per day, mean and max context tokens per
  request (input plus cache read plus cache write), cache-read cost per day, and requests per hour of
  activity as the closest thing to turns per session.
- `gain -by day` gets the columns; the overview gets one line: "N requests a day, mean context A tokens
  (max B)". The browser dashboard shows the same as a small chart. JSON fields and MCP `stats` follow `render.go` conventions.
- A session column is not possible from the proxy; do not add one.

## Acceptance
- Fixture ledger test: counts, mean, max, empty window, sessions with one request.
- The numbers match `replay` on the same transcripts within rounding; say how rounding is done in
  the `estimates` note.
