status: todo
kind: chore
commit: refactor: split oversized files
scope: cmd/gascoigne/**,internal/pricing/**,internal/mcptool/**,optimizer/logoptimizer/**,optimizer/catoptimizer/**
depends_on: 025-cmd-long-functions, 026-lib-long-functions, 027-optimizer-long-functions

# Split oversized files

`cmd`: `mcpreview.go` (HTTP handlers -> `mcpreview_http.go`), `eval.go` (-> `eval_report.go`), `replay.go`, `help.go` (tables vs rendering), `render.go`. `internal/pricing/pricing.go` (cache, loader, fetch, `SplitArgs`), `internal/mcptool/mcptool.go` (tools, sessions, stats, overview). `logoptimizer/log.go` (template, repeat, dedupe), `catoptimizer/outline.go` (one file per outline engine).

Move only, no edits inside. Run after 025-027 to avoid conflicts. Acceptance: `make check`.
