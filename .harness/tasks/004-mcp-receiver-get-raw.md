status: todo

# mcp receiver (stdio) with get_raw

## Goal
Expose gascoigne to agents as an MCP server: optimize text and fetch originals.

## Scope
- Package `internal/receiver/mcpreceiver`; hand-rolled JSON-RPC 2.0 over stdio (stdlib only),
  MCP methods: `initialize`, `tools/list`, `tools/call`, notifications ignored.
- Tools:
  - `optimize {text, kind?, attrs?, raw?}` → runs pipeline `mcp`; `raw: true` sets attr `raw=true`.
  - `get_raw {id}` → original text from ledger; marks key failure `raw_requested`.
- CLI `gascoigne mcp` runs pipeline `mcp` (add to `configs/default.yaml`).

## Acceptance
- Test driving the receiver with scripted JSON-RPC lines via `io.Pipe`.
- `get_raw` of an unknown id returns a JSON-RPC error, not a crash.
- `make check` green.
