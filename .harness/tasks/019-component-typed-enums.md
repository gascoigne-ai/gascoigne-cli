status: todo
kind: chore
commit: refactor(mcp): typed rule action and outcome notes
scope: receiver/mcpproxyreceiver/**,internal/mcptool/**,filter/**

# Typed enums in mcp and filters

- `mcpproxyreceiver/rules.go:12` `Action` bare string -> `type Action string`.
- `mcptool.outcomeNotes` (`mcptool.go:414`) keyed by `component.Outcome`.
- `mcptool.skipReasons` (`mcptool.go:423`) hardcodes filter names `minsize`, `bypass`, `history`: each filter records its own reason.

Run `SUITES=mcp make eval` too. Acceptance: `make check`.
