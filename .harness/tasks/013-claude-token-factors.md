status: todo

`tokens.Estimate` is calibrated to o200k (mean +-10%, worst 30% on tables, `tokens/tokens.go:9-12`).
Claude is priced with one global factor 1.58 (`internal/cost/cost.go:33`). The exact counter
`internal/exact/claude.go` exists, but the marker audit was never run with a key (backlog).

Scope: run it over the eval fixtures per dialect to get per-family factors (tables/JSON vs prose) and to
price the markers `§1`, `…`, `×N` and the hint line in Claude tokens. Then revisit `minSaving`
(`internal/service/pipeline.go:22`, 20 tokens: the hint may be about 32 Claude tokens) and hint wording.
Needs an API key from the user.

Done when: factors table committed with its generation command; `cost.go` uses it; `make check` and
`make eval` pass; `.harness/architecture/` cost doc updated in the same commit.
