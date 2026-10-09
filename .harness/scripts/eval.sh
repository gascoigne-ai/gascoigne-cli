#!/usr/bin/env bash
# Token savings benchmark: runs testdata/eval cases through the cli pipeline.
# Extra args go to `gascoigne eval` (e.g. -config my.yaml -pipeline cli).
set -euo pipefail
cd "$(dirname "$0")/../.."

go build -o bin/gascoigne ./cmd/gascoigne
exec bin/gascoigne eval -cases testdata/eval/cases.yaml "$@"
