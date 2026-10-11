status: todo

# Proxy: record instruction text in the ledger

## Goal
`gain` shows tool results and tool schemas, never the instruction text every request carries.

## Scope
- Mirror `recordTools` (`receiver/httpproxyreceiver/tools.go`): once per distinct block, one
  event, route `prompt`, command `system prompt` / `CLAUDE.md` / `skills` / `reminder`
  (by `<system-reminder>` content), outcome unchanged.
- Anthropic `system` + user text blocks; chat/responses `system`/`developer` roles; gemini
  `systemInstruction`.

## Acceptance
- Tests per API format; `gain -by command` lists the rows; `make check` green.
