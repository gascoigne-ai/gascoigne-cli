status: todo
# feat(truncate): error context and query selection

`truncate` keeps bare error lines with no context and ignores `query`
(`internal/optimizer/truncateoptimizer/truncate.go:57-68`). Backlog [16] also scores `max_tokens`
10000 -> 4000.

Scope: keep 1-2 lines after each kept error (the got/want line); when `query` is set, pick middle lines
by BM25 (`sample.Terms`). Evaluate the `max_tokens` change separately with an A/B.

Done when: `make check` and `make eval` pass; `.harness/architecture/` truncate section updated in the
same commit.
