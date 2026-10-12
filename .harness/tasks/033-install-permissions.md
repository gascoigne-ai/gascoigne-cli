status: todo

# install: allow rules for gascoigne's own tools

## Goal
Every user answers approval prompts for gascoigne's MCP tools (`retrieve`, `show`, `stats`,
`compress`, `fetch`). `install` should offer to add them to `permissions.allow`, record them, and
`unwrap` should remove exactly those.

## Scope
- Generalize `denies.go` to rule kinds (deny, allow): state files `denies.json`, `allows.json`,
  `addRules/recordRules/removeRules`; `unwrap` removes both kinds.
- Levels: `none` (default, nothing changes), `local` (retrieve, show, stats, compress: read-only,
  local), `all` (adds `mcp__gascoigne__fetch`, which reaches the network). Stored as `allow` in
  `prefs.json` (task 031 creates the file; create it here if this lands first).
- Flags `install -permissions none|local|all`; changing the level removes the old recorded rules first.
- Claude only (settings `permissions.allow`); other agents are out unless their config has an allow list.

## Acceptance
- Table tests: add, re-add (idempotent), level change, user's own identical rule left alone on
  unwrap, key order stays, backup kept.
- Architecture §Agents "Deny rules" renamed and extended.
