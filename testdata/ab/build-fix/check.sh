#!/bin/sh
# Solved: everything builds, the log engine's tests pass, no test file changed.
go build ./... >/dev/null 2>&1 && go test ./optimizer/logoptimizer/... >/dev/null 2>&1 && git diff --quiet -- '*_test.go'
