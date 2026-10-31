status: todo
# feat(table): make eval table dialect

`make eval` output in this repo is about 8.5k tokens per run (94k tokens a month unchanged): a table
nothing handles.

Scope: decide between (a) a generic fix, e.g. a dialect for the eval table shape in `table/aligned`
(header not upper-case, see `detect/table.go:39-62`), or (b) a project override through task 011.
Prefer (a) if the shape is common. Keep failing rows and the totals.

Done when: real output fixture; `make check` and `make eval` pass; architecture doc updated in the same
commit.
