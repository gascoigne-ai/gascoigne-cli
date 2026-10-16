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

declare -A cases=(
  [fixtures]=testdata/eval/cases.yaml
  [ext]=testdata/eval-ext/cases.yaml
  [scale]=testdata/eval-scale/cases.yaml
  [live]=testdata/eval/live.yaml
  [ledger]=
)
suites=${SUITES:-fixtures ext scale live ledger}

go build -o bin/gascoigne ./cmd/gascoigne || exit 1
failed=()
for s in $suites; do
  [[ -v cases[$s] ]] || { echo "unknown suite $s (${!cases[*]})" >&2; exit 2; }
  echo "== $s"
  if [[ $s == ledger ]]; then
    go test ./extension/ledger -run '^$' -bench . -benchtime 3x || failed+=("$s")
  else
    bin/gascoigne eval -cases "${cases[$s]}" "$@" || failed+=("$s")
  fi
done
if ((${#failed[@]})); then
  echo "eval failed: ${failed[*]}" >&2
  exit 1
fi
