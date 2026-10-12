status: todo

# Name the divergence point of a cache rewrite

## Goal
`gascoigne context` already says how much cache was written again instead of read and why by time: a
pause past the cache lifetime, a compaction, or "changed within the cache lifetime". The last kind is
the one to explain (about 4% of the cost on one real month: 19 requests of ~190k tokens). A transcript
cannot say what changed; the request bodies can.

## Scope
- `replay` (it rebuilds the requests of Claude Code transcripts): per request, find where its prefix first
  differs from the previous request: system prompt, tool set, message k, tool result k, or model.
  Report cache-write tokens by that point, with the top offenders.
- Join the result to the context page's "changed within the cache lifetime" row.
- `prompts` lint (second step): flag volatile text (dates, counters, ids) in instruction files and hook
  output that sits early in the prefix, and mid-session edits of instruction files.

## Acceptance
- Unit tests with two synthetic request bodies per cause.
- `replay -sessions 50` on real transcripts: every rewrite named, the unexplained share reported.
- Findings recorded in `backlog.md` (measured), including what could be fixed from our side.
