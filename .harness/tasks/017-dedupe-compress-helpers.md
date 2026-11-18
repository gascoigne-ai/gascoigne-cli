status: todo
kind: chore
commit: refactor(compress): share isWord and plural helpers
scope: internal/compress/**, internal/pcomb/**, optimizer/logoptimizer/**

# Dedupe small helpers

Real copies (three or more, or byte-identical):
- `isWord(char byte)`: `internal/compress/symbols/blocks.go`, `internal/pcomb/pcomb.go`,
  `optimizer/logoptimizer/categorical.go`.
- `plural`: `internal/compress/testreport/gotest.go` and `internal/compress/trace/trace.go`
  (identical); `diag/diag.go` has a different signature, leave it.

Steps:
1. Check for an import cycle first: `pcomb` importing `internal/compress` may not be allowed.
   If it is, export `isWord` from `pcomb` instead.
2. Add `compress.Count(count, word)` next to `compress.Omitted` and use it for `plural`.
3. Do not touch `human`/`size` (different units, output would change), `empty`/`split` (different types),
   or the `re` wrappers (they only shorten rule tables).

Acceptance: `make check` passes; `make eval` identical. Run after task 016 so `trace` has moved.
