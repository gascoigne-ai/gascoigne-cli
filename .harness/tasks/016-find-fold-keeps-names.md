status: todo

`fs/find` and `tree` fold past `max_per_dir: 12` into `… 16 more: (none) ×16`
(`internal/optimizer/lsoptimizer/paths.go:18-33,69`). The hidden entries were package/directory names, so
the agent needed a second query: cost greater than the saving.

Scope: never fold extension-less entries (directories/packages); fold files by extension only when 3 or
more share one; rename `(none)`. Real `find` output fixture.

Done when: `make check` and `make eval` pass with an intentionally updated baseline;
`.harness/architecture/families.md` fs section updated in the same commit.
