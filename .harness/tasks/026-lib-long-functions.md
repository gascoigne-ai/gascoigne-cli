status: todo
kind: chore
commit: refactor(lib): split long functions in libs and receivers
scope: internal/**,receiver/**,extension/**

# Split long functions in libs and receivers

`mcpproxyreceiver/proxy.go:144` `populate` (63, nesting 4; `listRecorder` + `registerTools`), `mcptool.BuildOverview` (66), `mcpreceiver/mcp.go:76` `fetch` (65), `extension/ledger/report.go:19` `liveStats`, `service/pipeline.go:79` `run` (stages), `jsonx/table.go:127` `hoist`, `testreport/gojson.go:29`, `diag/sarif.go:13`, `mcpauth/login.go:26`, `mcptool/attrs.go:16`, `symbols/symbols.go:208` and `blocks.go:23` (nesting 4-5), `httpproxyreceiver/formats.go:13` (nesting 4).

Tests unchanged. Also run `SUITES=mcp make eval`. Acceptance: `make check`.
