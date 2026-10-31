status: todo
# feat(gain): rerequest rate columns

`gain` nets give-back into every grouping (`extension/ledger/report.go:108`) but shows no rate, and
`Impact` is per day only (`report.go:341-353`). There is no way to see which dialect or optimizer is too
lossy (causes `raw`/retrieve re-reads). Partial `raw` reads (`-grep/-lines/-query`) are not recorded as an
outcome, so they are only visible in transcripts.

Scope: add `rerequested`, `wasted` and a rerequest % column to `-by dialect|optimizer|command`, with a
sort on it. Record partial `raw` reads as an outcome.

Done when: report tests; `make check` passes; `.harness/architecture/` ledger/report doc updated in the
same commit. This is the dashboard used to choose later dialect work.
