status: todo

# Attribute prompt-cache writes to a cause

## Goal
30 days of real sessions: 3.5M cache-write tokens, priced about 12x a read. Nobody knows what
causes them. Name the first point where each request's prefix differs from the previous request.

## Scope
- `replay` (and the proxy's usage meter when it can see both requests): per request, find the
  divergence point: system prompt, tool set, message k, tool result k, or "TTL expired".
- Report in `replay`: cache-write tokens by cause, top offenders (hook context placed early,
  mid-session instruction edits, changing tool sets, volatile text).
- Dashboard row and `gain` line only after the replay report proves useful.
- `prompts` lint (same task, second step): flag volatile text (dates, counters, ids) in
  instruction files and hook output that sits early in the prefix.

## Acceptance
- Unit tests with two synthetic request bodies per cause.
- `replay -sessions 50` on real transcripts: every cache write is named; unexplained share
  reported, not hidden.
- Record findings in `backlog.md` (measured), including what could be fixed from our side.
