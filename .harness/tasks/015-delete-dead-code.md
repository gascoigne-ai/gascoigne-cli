status: todo
kind: chore
commit: chore(compress): delete dead code and unexport test-only helpers
scope: internal/compress/**, internal/detect/**

# Delete dead code

Audit (2026-11-18, grep only, no dead-code tool) found exported code with no production caller.

Steps:
1. Re-verify each item with grep across the repo, then remove:
   - `internal/compress/jstest.go` (`JSTest`) and `TestJSTest`: jest/vitest/bun run through `testreport` rules.
   - `compress.DedupeAll` (`internal/compress/compress.go`) and its test.
   - `detect.Families` (`internal/detect/families.go`): no references.
2. Move helpers used only by tests into `_test.go`: `detect.Types`, `detect.IsRow`, `diag.Dialects`,
   `testreport.Dialects` (generic.go).
3. Unexport `compose.As` (no caller outside the package).
4. Check with grep, then unexport if used only in their own package: `pkgnoise.Fold`, `symbols.Def`,
   `symbols.ErrLang`, `imgscale.PixelsPerToken`, `tree.Legend`, `drain.Miner`. Do not unexport when a
   returned type must stay public.

Acceptance: about 120 lines removed, no test changed except the deleted ones, `make check` passes,
`make eval` output identical.
