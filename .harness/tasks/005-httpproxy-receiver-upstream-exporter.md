status: todo

# httpproxy receiver + upstream exporter

## Goal
LLM API proxy: agent points its base URL at gascoigne; tool results in requests get optimized.

## Scope
- `internal/receiver/httpproxyreceiver`: `net/http` server (`listen` config). Parses Messages
  API style request bodies; each `tool_result` content block → payload block (`tool_result` kind,
  `tool` attr from matching tool_use name). System prompt and last user message untouched.
- Header `X-Gascoigne-Raw: 1` → attr `raw=true`. `GET /raw/<id>` serves originals (marks failure).
- `internal/exporter/upstreamexporter`: forwards rewritten body via `httputil.ReverseProxy`
  to configured `url`; streams responses untouched.
- Pipeline `proxy` with its own `router` override in `configs/default.yaml`.

## Acceptance
- `httptest` upstream asserting rewritten body; headers (auth) forwarded unchanged.
- Streaming (SSE) response passes through byte-identical.
- `make check` green.
