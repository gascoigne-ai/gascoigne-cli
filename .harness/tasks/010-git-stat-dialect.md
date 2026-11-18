status: todo
kind: feature
commit: feat(vcs): route merge, rebase, cherry-pick, commit, checkout and stash pop
scope: internal/detect/vcs.go, internal/detect/rows_*.go, optimizer/**/vcs*, testdata/eval/**, .harness/architecture/families.md

# Git stat-output dialect

`git merge`, `rebase`, `cherry-pick`, `commit`, `checkout` and `stash pop` have no route. `git merge
--ff-only` falls to `doc/text`: 33 events, 140k tokens unchanged (about 4.2k each), only `truncate` fires
above 10k. Add dialect rows for them and reuse the existing stat folding (`statEnd`: bars to counts)
plus a fold for `create mode` / `delete mode` runs.

Test first: add cases to `testdata/eval/cases.yaml` with real output (a ff merge with a large stat, a
rebase, a cherry-pick, a commit with many created files). Conflict markers, `CONFLICT` lines and error
text must survive.

Acceptance: `make eval` shows the new cases saved and `raw ok` never `NO`; pin the new numbers in
`testdata/eval/baseline.tsv`; `make check` passes. Keep the work under the stop rule: drop it if the
cases show under 40% saved.

Model: unchanged
