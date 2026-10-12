status: todo

# Record MCP tool use and tool-set size (14 days)

## Goal
First step of the weekly MCP review (tasks 055 and 056). Tool schemas and descriptions are re-sent
with every request, and nobody knows which servers and tools earn that cost. Record who uses what,
cheaply, and keep two weeks so a week can be compared with the one before.

## Scope
- Two sources, both already on the path, none needing the LLM proxy:
  - `mcp-proxy` (`receiver/mcpproxyreceiver`) sees `tools/list` and every `tools/call` of the stdio
    servers it wraps: per tool, schema tokens and calls. This is the exact source.
  - The PostToolUse / AfterTool hooks (`mcp__*`, `mcp_*`) see calls to servers that stay direct
    (remote ones) for claude, codex and gemini: calls only, no sizes. Counting happens in the
    background server, not in the hook (no ledger open in the hot path); the hook never fails.
- Two small daily tables in the ledger (additive, `AutoMigrate`): `tool_use` (day, agent, server,
  tool, calls) and `tool_set` (day, agent, server, tools, schema tokens, tool names with sizes).
  Rows are aggregated per day, so a year is a few thousand rows.
- Retention: a new ledger config key `stats_ttl`, default 14 days (336h), pruned next to `raw`.
  `raw_ttl` stays 24h: originals are the bulk of the disk and the review does not need them.
  Events and usage rows are not pruned today and stay so.
- `reset` and `flush` handle the new tables; `docs/ledger.md` and the `architecture.md` Ledger
  section list them.
- Server and tool names only. Never arguments, results or paths.

## Acceptance
- Fixture tests: counts per day, aggregation of repeated calls, prune at the TTL, a hook call with
  no matching list (calls kept, size unknown), concurrent writers.
- The per-server schema tokens match what `gain -by route` reports for route `tools` on the same
  fixture, within the tokenizer's estimate error.
- No new dependency; no change to `component/`, `payload/` or `internal/` core (ask first if one
  is needed).
