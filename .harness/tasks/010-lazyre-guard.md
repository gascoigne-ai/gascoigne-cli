status: todo
kind: chore
commit: build: make check rejects package-level regexp.MustCompile
scope: Makefile, internal/prose/runtime.go, cmd/gascoigne/render_test.go

# Enforce lazyre.New for package-level regexps

CLAUDE.md says package-level regexps use `lazyre.New`, but nothing checks it and two sites break it (`internal/prose/runtime.go`, `cmd/gascoigne/render_test.go`).

Model: unchanged.

## Acceptance
- Test first: the tests that state the behavior, run and seen failing for the expected reason. Add the Makefile grep guard first and see it fail on the two sites.
- Move both sites to `lazyre.New`.
- `make check` passes.
