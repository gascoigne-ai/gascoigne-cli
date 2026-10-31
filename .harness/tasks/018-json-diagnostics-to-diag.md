status: todo
# feat(diag): JSON diagnostics from linters and compilers

JSON diagnostics (`cargo build --message-format=json`, `eslint -f json`, `ruff --output-format json`,
`golangci-lint --out-format json`, `mypy --output json`, `pylint -f json`) hit the ndjson/json crusher,
which samples records. That is lossy for diagnostics. Text form is what the agent wants.

Scope: generalize `internal/compress/diag/sarif.go` into a table of JSON shapes -> diag records ->
existing renderer. Hook: `detect/build.go:26` (SARIF spec pattern), new Rows. One shape per commit.
Real tool output fixtures.

Done when: `make check` and `make eval` pass; diagnostics lost never above zero in the probe;
`.harness/architecture/families.md` build section updated in the same commit.
