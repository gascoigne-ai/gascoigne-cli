status: todo

# mcp-proxy: lazy tool exposure

## Goal
Agents without native tool search pay for every downstream schema on every request.

## Scope
- When downstream tools exceed `max_tools` or `max_schema_tokens` (config, off by default),
  expose `search_tools(query)` (BM25 over names/descriptions, returns schemas) and
  `call_tool(name, args)` instead.
- Not enabled for claude by `wrap`/`install`.

## Acceptance
- Tests with a fake downstream of 50 tools; `make check` green.
