status: todo

# Use the agent's stated intent as the query for a tool result

## Goal
`truncate`, `json` sampling, `fetch` and `aria` already keep what matches a query, but the proxy
only has `query` or `prompt` when the tool call carries one. The assistant text just before a tool
call usually says what it looks for. Use it.

## Scope
- `httpproxyreceiver` `attrsOf`: when a tool call has no query of its own, take the nearest
  preceding assistant text block (capped, about 200 characters, stop words dropped) as the `query`
  attr of that result. Deterministic for a given transcript, so the result cache and the prompt
  cache stay stable.
- Never for `skip` tools or results already hinted; never changes which optimizer runs.
- Off switch in `receivers.httpproxy` config (`intent_query: false`); on only after the measurement.

## Acceptance
- Table tests: text before the call is used, none present, long text cut, same transcript gives the
  same bytes.
- `make eval` fixtures with a query prove the ranking effect; `replay` shows byte-identical
  re-sent messages stay identical.
- `ab.sh`: success stays 100%; tokens per task drop on tasks that read large outputs. Record in
  `backlog.md`.
