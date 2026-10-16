#!/usr/bin/env bash
# Token savings benchmark: every eval suite through the cli pipeline, one summary at the end.
#   fixtures  testdata/eval/cases.yaml        fixture files, goldens, baseline
#   ext       testdata/eval-ext/cases.yaml    detection of extension-only formats
#   scale     testdata/eval-scale/cases.yaml  big generated outputs, max_ms ceilings
#   live      testdata/eval/live.yaml         real commands in this repo (tool output drift)
#   ledger    go benchmarks                   rollup and Stats on a seeded 20k-run ledger
# SUITES="scale live" picks some. Extra args go to every `gascoigne eval` run
# (e.g. -update, -show rg, -config my.yaml, -pipeline cli). Opt-in features have their own
# run: `make eval-experimental`.
set -uo pipefail
cd "$(dirname "$0")/../.."

# casesfile prints a suite's cases file (empty for ledger) and fails for an unknown suite.
# A case statement, not an associative array: macOS ships bash 3.2, which has none.
casesfile() {
  case $1 in
    fixtures) echo testdata/eval/cases.yaml ;;
    ext) echo testdata/eval-ext/cases.yaml ;;
    scale) echo testdata/eval-scale/cases.yaml ;;
    live) echo testdata/eval/live.yaml ;;
    ledger) echo ;;
    *) return 1 ;;
  esac
}
suites=${SUITES:-fixtures ext scale live ledger}

go build -o bin/gascoigne ./cmd/gascoigne || exit 1
failed=()
for s in $suites; do
  file=$(casesfile "$s") || { echo "unknown suite $s (fixtures ext scale live ledger)" >&2; exit 2; }
  echo "== $s"
  if [[ $s == ledger ]]; then
    go test ./extension/ledger -run '^$' -bench . -benchtime 3x || failed+=("$s")
  else
    bin/gascoigne eval -cases "$file" "$@" || failed+=("$s")
  fi
done
if ((${#failed[@]})); then
  echo "eval failed: ${failed[*]}" >&2
  exit 1
fi
