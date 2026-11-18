status: todo
kind: chore
commit: refactor(compress): split the pkgnoise rule table and diag.Compress
scope: internal/compress/pkgnoise/**, internal/compress/diag/**, .harness/architecture/families.md

# Split pkgnoise table and diag.Compress

Steps:
1. `internal/compress/pkgnoise/pkgnoise.go` is 426 lines, about 350 of them the rule table. Move the table
   to `dialects.go`, as `diag` already does.
2. `internal/compress/diag/diag.go` `Compress` is 95 lines. Split into named phases (parse, group,
   render) with the same order and data. Pin current output first with a test over the existing fixtures
   if coverage is thin (code without tests is pinned before it is changed).
3. Optionally: `htmlx/writer.go` `node` (74 lines) as an atom handler table; `logoptimizer/log.go`
   `template` (74 lines).

Medium risk: eval-sensitive. Acceptance: `make check` passes; `make eval` savings do not regress, `raw ok`
is never `NO`, output byte-identical on the eval fixtures.
