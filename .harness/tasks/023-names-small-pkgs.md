status: todo
kind: chore
commit: refactor(small): full parameter names in small-pkgs
scope: receiver/[a-g]*/**, receiver/[i-z]*/**, filter/**, exporter/**, component/**, payload/**, tokens/**, configs/**, inproc/**, cmd/mcpmock/**
depends_on: 014-param-name-check

# Full parameter names in small-pkgs

Rename one-letter parameters and closure arguments to names of the domain (`request`, `message`). Behavior does not change; no test changes.

Model: unchanged.

## Acceptance
- `go run ./cmd/paramnames` over this scope reports nothing.
- Tests are unchanged.
- `make check` passes.
