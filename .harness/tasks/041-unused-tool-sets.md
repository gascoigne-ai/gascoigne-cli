status: todo

# Report MCP servers and tools that cost tokens and are never used

## Goal
Tool schemas and descriptions are re-sent with every request (~35k tokens, cached but re-read each
turn). Users often keep MCP servers installed that no call used in a month. Show the cost per
server and let the user act.

## Scope
- Ledger side: route `tools` events hold the tool set size; MCP call events hold the tool used.
  Join them per server: tools, tokens per request, calls in the window (`-since`, default 30d).
- `gain` section "tool sets" (or `gain -by tools`), JSON field, MCP `stats`; `doctor` lists servers
  with 0 calls and their per-request cost.
- Setup question (`decision` in `setup.go`, optional group) offering to disable a server per
  project. Nothing changes without an answer (decisions.md).
- No core change; if the ledger needs a column, ask first (AutoMigrate rules in architecture.md).

## Acceptance
- Fixture ledger test: server with calls vs without, window filter, per-request token figure
  matches `gain -by route` for route `tools`.
- Terminal output follows `render.go` conventions; estimates note updated.
