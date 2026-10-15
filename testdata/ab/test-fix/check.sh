#!/bin/sh
# Solved: the package's tests pass and no test file changed.
go test ./internal/compress/probe/ >/dev/null 2>&1 && git diff --quiet -- '*_test.go'
