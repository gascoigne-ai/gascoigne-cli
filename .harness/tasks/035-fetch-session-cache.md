status: todo
# feat(mcp): fetch session cache

Backlog [16]: the `fetch` tool refetches and re-optimizes the same URL in a session. Repeat fetches should
return `unchanged since <id>` or a diff.

Scope: `receiver/mcpreceiver/mcp.go:75-113`; key by `url` plus session via the ledger raw lookup; when the
body hash matches return a pointer, otherwise a line diff capped by size. `raw: true` bypasses. Mind task
002: a failed key must not decay into permanent raw.

Done when: `make check`, `make eval` and `make eval-mcp` pass; `.harness/architecture/` mcp doc updated in
the same commit.
