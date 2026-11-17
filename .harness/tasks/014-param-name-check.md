status: todo
kind: feature
commit: build: paramnames reports one-letter parameters
scope: cmd/paramnames/**
depends_on: 009-layer-rows

# paramnames checker

Add `go run ./cmd/paramnames <paths>`: with `go/ast` report one-letter function, method and closure parameters. Exempt `t`/`b` of testing types, `_`, Go method receivers and type parameters. Print counts per package. Not part of the check yet.

## Model
A violation is (file, line, name, package). Output sorted by package; no persistence.

## Acceptance
- Test first: the tests that state the behavior, run and seen failing for the expected reason. Table over source snippets: one bad and one allowed case per exemption.
- Record the real per-package counts in the commit body.
- `make check` passes (the task touches code, so the check runs).
