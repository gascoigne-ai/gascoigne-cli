status: todo
kind: chore
commit: refactor(cmd): split long functions, flatten nesting
scope: cmd/gascoigne/**

# Split long functions in cmd

Over ~45 lines: `cmdTokens` (62), `cmdLearn`, `cmdPrompts`, `diagnose`, `saveSetup`, `cmdInstall`, `cmdEval`, `runHandlers`, `cmdWrap`, `promptFindings`, `pricing apply`, `proxiedConfigs`, `loadContext`. Split flag parsing from the action.

Nesting over two levels: `continuity.go:77-99` (`applyBlock`), `replay.go:462` (`toolResultText`), `proxy.go:281`, `unwrap.go:317`, `codextoml.go:117`, `prompts.go:368` (early `continue`). Pure data tables in `help.go` stay.

Tests unchanged. Acceptance: `make check`.
