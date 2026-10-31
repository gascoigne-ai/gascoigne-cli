status: todo

Coverage is handled only for `go tool cover -func` and lcov. Others (`pytest --cov` term report
`Name Stmts Miss Cover`, `jest --coverage` boxed table, nyc/c8, simplecov, `coverage report`) go to
`table/boxed`, which cuts at 30 arbitrary rows and ignores `test.coverage_below`
(`configs/default.yaml:43`).

Scope: extend `internal/compress/testreport/coverage.go` and `detect/test.go:33`; list only files under
the threshold plus the total. Real output fixtures.

Done when: `make check` and `make eval` pass, 70%+ saved; `.harness/architecture/families.md` test
section updated in the same commit.
