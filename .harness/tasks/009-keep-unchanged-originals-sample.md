status: todo
kind: feature
commit: feat(ledger): keep a capped sample of unchanged originals
scope: extension/ledger/**, cmd/gascoigne/probe*.go, .harness/architecture/ledger.md

# Sample unchanged originals, export them as scrubbed fixtures

Only 43 of 8258 `unchanged` events have a `raw` row, so there is no real output to build fixtures for
the dialects that save 0% (`search/git-grep`, `table/ss`, `table/sys`, `build/fmt`, `build/php`,
`test/catch`, `test/minitest`) or for the commands with no routes (cargo tree, gradle dependencies,
composer, yarn why, terraform, pulumi, kubectl, aws, gh).

Add:
- A size-capped sample of unchanged originals per detected type (config: bytes per type and total
  cap, default small). Oldest sample is evicted first.
- A `gascoigne probe` export of the top unchanged originals per type, with home paths, usernames,
  emails and tokens scrubbed. The scrubber must fail closed: an original it cannot scrub is skipped.

Test first: sampling cap and eviction; scrub tests with real-shaped secrets and paths.

Acceptance: export runs on a throwaway ledger and writes fixtures that pass `make eval` once cases are
added. Nothing in the export contains a home path. `make check` passes.

## Model
New ledger table for sampled originals (type, bytes, ts, body) with a per-type and total byte cap, oldest evicted first.
