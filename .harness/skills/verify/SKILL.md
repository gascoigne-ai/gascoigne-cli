---
name: verify
description: Verify gascoigne changes before calling work done. Use after any code edit, before committing, or when asked to check/test the repo.
---

# Verify

1. Run `make check` (gofmt check, `go vet`, `go test ./...`).
2. On failure: read the first error, fix the cause (not the test), rerun. Repeat until green.
3. If you touched an optimizer, router, filter or `configs/default.yaml`: run `make eval`.
   - `raw ok` column must never show `NO`.
   - Compare `saved %` to the previous run; explain any regression or fix it.
4. Report what ran and the result. Never claim green without running it in this session.

Never: skip or delete failing tests, add `t.Skip`, loosen assertions to pass, or
`//nolint`/ignore vet findings without the user's approval.
