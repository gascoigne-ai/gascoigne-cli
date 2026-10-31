status: todo
# feat(raw): size header on large originals

73% of `gascoigne raw` calls in local transcripts read the whole original (107 full vs 39 partial).
Each is an extra turn plus the full text.

Scope: `raw` prints a one-line header for originals over about 2k tokens ("N tokens; `-grep`,
`-lines A-B`, `-query` read part") before the text. The full text is still printed, so the
"original always retrievable" guarantee holds. The hint already carries `A→B` sizes.

Done when: test; compare the full-vs-partial ratio with task 008 after a week; `make check` passes;
`.harness/architecture/` raw doc updated in the same commit.
