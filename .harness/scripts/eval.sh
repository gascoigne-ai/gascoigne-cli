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

# Banner styling: colors only on a terminal without NO_COLOR.
if [[ -t 1 && -z ${NO_COLOR:-} ]]; then B=$'\e[1m' D=$'\e[2m' C=$'\e[36m' R=$'\e[0m'; else B= D= C= R=; fi

# describe prints what a suite tests and how to read its result (bash 3.2: a case, no arrays).
describe() {
  local title tests means
  case $1 in
    fixtures)
      title="Fixtures - real tool output, before and after"
      tests="Captured output of hundreds of cases across tools (go, pytest, git, rg, npm, ...) goes through the cli pipeline."
      means="Detected type is right, key facts survive, savings stay above the floor, output matches the golden, raw comes back byte-exact. A FAIL means an optimizer change lost or altered something." ;;
    ext)
      title="Ext - needles in a haystack"
      tests="External-style dataset: JSON arrays with planted errors/anomalies, small mixed tool outputs."
      means="Savings must not cost the facts you would ask about. A FAIL means a planted needle was compressed away." ;;
    scale)
      title="Scale - big outputs, time limits"
      tests="Generated outputs up to 2M tokens (logs, find, ndjson, csv)."
      means="Savings hold at size and each case stays under its max_ms ceiling. A FAIL means a slowdown or a savings drop at volume." ;;
    live)
      title="Live - real commands in this repo"
      tests="go test, git log/diff, rg, find, ls, ... run now, not replayed."
      means="Catches stale fixtures: a wrong type or a savings drop here, while Fixtures still pass, means a tool changed its output shape. Missing tools are skipped." ;;
    ledger)
      title="Ledger - bookkeeping speed"
      tests="Go benchmarks: rollup and Stats on a seeded 20k-run ledger."
      means="Not pass/fail; compare ns/op between runs to spot regressions in the dashboard's data path." ;;
  esac
  local read="saved = tokens removed (bar: 10 cells = 100%); lost = error lines/artifacts dropped, as errors/artifacts, held at the baseline; ms = optimize time."
  [[ $1 == ledger ]] && read="ns/op = time per operation, lower is better."
  printf '\n%s== %s%s\n%s   Tests: %s\n   Means: %s\n   Read:  %s%s\n\n' \
    "$B$C" "$title" "$R" "$D" "$tests" "$means" "$read" "$R"
}

go build -o bin/gascoigne ./cmd/gascoigne || exit 1
failed=()
for s in $suites; do
  file=$(casesfile "$s") || { echo "unknown suite $s (fixtures ext scale live ledger)" >&2; exit 2; }
  describe "$s"
  if [[ $s == ledger ]]; then
    go test ./extension/ledger -run '^$' -bench . -benchtime 3x || failed+=("$s")
  else
    bin/gascoigne eval -cases "$file" "$@" || failed+=("$s")
  fi
done
printf '\n%sSummary%s\n' "$B" "$R"
for s in $suites; do
  case " ${failed[*]:-} " in
    *" $s "*) printf '  %-9s FAIL\n' "$s" ;;
    *) printf '  %-9s ok\n' "$s" ;;
  esac
done
if ((${#failed[@]})); then
  echo "eval failed: ${failed[*]}" >&2
  exit 1
fi
