status: todo

The PostToolUse hook gives the pipeline only `AttrTool` (`cmd/gascoigne/posthook.go:61-62`). The proxy
also sets `query` (query/q/search/prompt, or `Intent(description)`) and `url`
(`receiver/httpproxyreceiver/results.go:68-122`, `attrsOf`). Without them, fetch-like MCP results are
not typed web/http and BM25 sampling never runs. Login users (no proxy) are the common case.

Scope: move `attrsOf` into `internal/mcptool` and call it from both the proxy and `posthook.go`. Fixtures
from real MCP tool inputs in `testdata/eval-mcp`.

Done when: `make check`, `make eval`, `make eval-mcp` pass; savings on MCP cases do not regress;
`.harness/architecture/hooks.md` updated in the same commit.
