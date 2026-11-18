status: todo
kind: chore
commit: refactor(cmd): parse tool and message JSON into types once
scope: cmd/gascoigne/**,receiver/httpproxyreceiver/**

# Typed JSON at the boundary

`cmd/gascoigne/posthook.go:75-290` (12 `.(map[string]any)`), `replay.go` (`messages []map[string]any`), `billing.go:76` `readJSON`: decode into small structs once. `httpproxyreceiver` (`formats.go`, `tools.go`, `skills.go`, `prompts.go`, `usage.go`): it must keep unknown fields, so add small typed accessors (`str`, `obj`) rather than structs.

Tests unchanged. Acceptance: `make check`.
