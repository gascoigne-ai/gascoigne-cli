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
    mcp) echo testdata/eval-mcp/cases.yaml ;;
    live) echo testdata/eval/live.yaml ;;
    ledger) echo ;;
    *) return 1 ;;
  esac
}
suites=${SUITES:-fixtures ext scale mcp live ledger}

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
    mcp)
      title="MCP - results of a fake MCP server"
      tests="Fake-data tool results (issues, code hits, logs, rows, pages) fetched over the MCP protocol from cmd/mcpmock, through the mcp-proxy pipeline."
      means="MCP results shrink by the floor and small ones stay untouched. A FAIL means the proxy path lost savings or a fact on MCP output." ;;
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

go build -o bin/gascoigne ./cmd/gascoigne && go build -o bin/mcpmock ./cmd/mcpmock || exit 1
failed=()
results=$(mktemp)
trap 'rm -f "$results"' EXIT
for s in $suites; do
  file=$(casesfile "$s") || { echo "unknown suite $s (fixtures ext scale mcp live ledger)" >&2; exit 2; }
  describe "$s"
  out=$(mktemp)
  if [[ $s == ledger ]]; then
    go test ./extension/ledger -run '^$' -bench . -benchtime 3x 2>&1 | tee "$out"
  else
    pipeline=cli
    [[ $s == mcp ]] && pipeline=mcp-proxy
    # tee hides the terminal from eval: ask for the colors it would show on a terminal
    CLICOLOR_FORCE=${B:+1} bin/gascoigne eval -pipeline "$pipeline" -cases "$file" "$@" 2>&1 | tee "$out"
  fi
  status=${PIPESTATUS[0]}
  [[ $status -ne 0 ]] && failed+=("$s")
  # one line per suite: name, cases, failed cases, raw tokens, out tokens, saved %
  sed $'s/\e\\[[0-9;]*m//g' "$out" | awk -v s="$s" '
    $1 == "total" && NF >= 5 { raw = $2; opt = $3; pct = $4 }
    /^[0-9]+ cases:/ { cases = $1; if (match($0, /[0-9]+ FAILED/)) bad = substr($0, RSTART, RLENGTH - 7) }
    END { printf "%s\t%s\t%s\t%s\t%s\t%s\n", s, cases + 0, bad + 0, raw + 0, opt + 0, pct }' >>"$results"
  rm -f "$out"
done
# bar <percent>: 10 cells, one per 10%.
bar() {
  local filled=$(( ($(printf '%.0f' "$1") + 5) / 10 )) out="" i
  ((filled > 10)) && filled=10
  for ((i = 0; i < 10; i++)); do ((i < filled)) && out+="█" || out+="░"; done
  printf '%s' "$out"
}
if [[ -n $B ]]; then G=$'\e[32m' RD=$'\e[31m' Y=$'\e[33m'; else G= RD= Y=; fi
rule=$(printf '─%.0s' {1..70})
printf '\n%sEval summary%s\n%s%s%s\n' "$B$C" "$R" "$D" "$rule" "$R"
printf '%s  %-9s %-7s %9s %12s %12s   %-6s %s%s\n' "$B" suite result cases "tokens in" "tokens out" saved "" "$R"
sum=0 n=0 okc=0 total=0
while IFS=$'\t' read -r s cases bad raw opt pct; do
  total=$((total + 1))
  case " ${failed[*]:-} " in
    *" $s "*) res="${RD}✗ FAIL${R}" ;;
    *) res="${G}✓ ok${R}  "; okc=$((okc + 1)) ;;
  esac
  if [[ $s == ledger ]]; then
    printf '  %-9s %s %9s %12s %12s   %s%s%s\n' "$s" "$res" - - - "$D" "benchmarks, compare ns/op" "$R"
    continue
  fi
  ((bad)) && cases="$((cases - bad))/$cases"
  num=${pct%\%}
  if awk -v p="${num:-0}" 'BEGIN { exit !(p >= 60) }'; then tint=$G; elif awk -v p="${num:-0}" 'BEGIN { exit !(p >= 30) }'; then tint=$Y; else tint=$RD; fi
  printf '  %-9s %s %9s %12s %12s   %s%6s %s%s\n' "$s" "$res" "$cases" "$raw" "$opt" "$tint" "${pct:--}" "$(bar "${num:-0}")" "$R"
  if [[ -n $num ]]; then
    sum=$(awk -v a="$sum" -v p="$num" 'BEGIN { print a + p }')
    n=$((n + 1))
  fi
done <"$results"
printf '%s%s%s\n' "$D" "$rule" "$R"
mean=$(awk -v a="$sum" -v n="$n" 'BEGIN { printf "%.1f", n ? a / n : 0 }')
if ((okc == total)); then verdict="${G}${B}all $total suites pass${R}"; else verdict="${RD}${B}$((total - okc)) of $total suites failed${R}"; fi
printf '  %s  ·  %smean saved %s%%%s over %d token suites\n' "$verdict" "$B" "$mean" "$R" "$n"
if ((${#failed[@]})); then
  echo "eval failed: ${failed[*]}" >&2
  exit 1
fi
