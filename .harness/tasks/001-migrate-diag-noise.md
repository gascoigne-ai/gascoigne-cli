status: todo

Move the remaining regex rule tables to `internal/pcomb`: `internal/compress/testreport` (`pass`,
`noise`, `frames`, `keep` per runner in `generic.go`), then `internal/compress/detect`-style tables
if a table reads as a long alternation. `diag` and `pkgnoise` noise are done.

Scope: one runner per commit, one rule per tool behavior, `Yes`/`No` lines from real tool output,
`Re` only for digits, repeats and unanchored shapes. Add a `Verify` test like `TestNoiseRulesVerify`.

While there, decide on quirks the first migration kept byte-for-byte (listed so nobody trips on them):
- `diag` meson: `^Message:\b` never matches real `Message: hello` (a boundary after `:` needs a word
  character next), so meson messages are not dropped.
- `pkgnoise` os/dnf: `Running scriptlet\s+:` never matches dnf's `Running scriptlet: pkg`.
Fixing either changes output: run `make eval` and update the baseline in the same commit.

Done when: `make check` and `make eval` show unchanged (or intentionally improved) savings and
`raw ok` never `NO`.
