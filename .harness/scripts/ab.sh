#!/usr/bin/env bash
# Task success with and without gascoigne: each task in testdata/ab runs with
# Claude Code headless twice per repeat, "off" (plain claude: user settings,
# MCP servers and GASCOIGNE_*/ANTHROPIC_BASE_URL dropped) and "on" (gascoigne
# wrap claude: hooks, proxy, MCP), in a fresh worktree of the tasks' base
# commit with the task's setup.patch applied. The task's check.sh decides
# success ($1 is a file with the agent's final answer). Prints pass rate,
# cost and tokens per arm; rows go to ab-results.tsv in the cache dir.
# This calls the API and costs money: -n checks the tasks without it (each
# must apply cleanly and fail its check before an agent works on it).
# Usage: .harness/scripts/ab.sh [-n] [-r repeats=3] [-m model] [task...]
set -euo pipefail
cd "$(dirname "$0")/../.."

dry=0 repeats=3 model=${AB_MODEL:-claude-sonnet-5-5} budget=${AB_BUDGET_USD:-2}
while getopts nr:m: o; do
	case $o in
	n) dry=1 ;;
	r) repeats=$OPTARG ;;
	m) model=$OPTARG ;;
	*) exit 2 ;;
	esac
done
shift $((OPTIND - 1))
tasks=("$@")
[ ${#tasks[@]} -gt 0 ] || mapfile -t tasks < <(find testdata/ab -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort)
base=$(cat testdata/ab/BASE)

go build -o bin/gascoigne ./cmd/gascoigne
gascoigne=$PWD/bin/gascoigne
out=${XDG_CACHE_HOME:-$HOME/.cache}/gascoigne/ab
mkdir -p "$out"
results=$out/ab-results.tsv
tmp=$(mktemp -d)
trap 'git worktree prune; rm -rf "$tmp"' EXIT

# clean runs a command without any trace of gascoigne from this shell.
clean() {
	local unset=(-u ANTHROPIC_BASE_URL)
	for v in $(compgen -e | grep '^GASCOIGNE_' || true); do unset+=(-u "$v"); done
	env "${unset[@]}" "$@"
}

# setup makes a worktree of the base commit with the task's bug planted.
setup() {
	local task=$1 dir=$2
	git worktree add -q --detach "$dir" "$base"
	[ ! -f "testdata/ab/$task/setup.patch" ] || git -C "$dir" apply "$PWD/testdata/ab/$task/setup.patch"
}

check() {
	local task=$1 dir=$2 answer=$3
	(cd "$dir" && sh "$OLDPWD/testdata/ab/$task/check.sh" "$answer")
}

if [ $dry = 1 ]; then
	for task in "${tasks[@]}"; do
		dir=$tmp/$task
		setup "$task" "$dir"
		: > "$tmp/empty"
		if check "$task" "$dir" "$tmp/empty"; then
			echo "$task: FAIL, solved before any agent ran"
		else
			echo "$task: ok (applies, unsolved)"
		fi
		git worktree remove --force "$dir"
	done
	exit 0
fi

flags=(-p --output-format json --model "$model" --max-budget-usd "$budget"
	--setting-sources project,local --strict-mcp-config
	--permission-mode acceptEdits --allowedTools "Bash Read Edit Write Grep Glob")
[ -f "$results" ] || printf 'time\ttask\tarm\tpass\tcost_usd\tinput\tcache_read\tcache_write\toutput\tturns\tseconds\n' > "$results"
for ((rep = 1; rep <= repeats; rep++)); do
	for task in "${tasks[@]}"; do
		prompt=$(cat "testdata/ab/$task/prompt.txt")
		for arm in off on; do
			dir=$tmp/$task-$arm
			setup "$task" "$dir"
			cmd=(claude)
			[ $arm = on ] && cmd=("$gascoigne" wrap claude)
			(cd "$dir" && clean "${cmd[@]}" "${flags[@]}" "$prompt" > "$tmp/run.json" 2> "$tmp/run.err") || true
			jq -r '.result // ""' "$tmp/run.json" > "$tmp/answer" 2>/dev/null || : > "$tmp/answer"
			pass=0
			check "$task" "$dir" "$tmp/answer" && pass=1
			jq -r --arg t "$task" --arg a "$arm" --arg p $pass '[now|todate, $t, $a, $p, .total_cost_usd, .usage.input_tokens,
				.usage.cache_read_input_tokens, .usage.cache_creation_input_tokens, .usage.output_tokens, .num_turns,
				(.duration_ms/1000|floor)] | @tsv' "$tmp/run.json" >> "$results" ||
				printf '%s\t%s\t%s\t0\t\t\t\t\t\t\t\n' "$(date -Is)" "$task" "$arm" >> "$results"
			echo "$task $arm #$rep: pass=$pass"
			git worktree remove --force "$dir"
		done
	done
done

echo
awk -F'\t' 'NR > 1 { n[$3]++; p[$3] += $4; c[$3] += $5; i[$3] += $6 + $7 + $8; t[$3] += $10 }
	END { printf "arm\truns\tpass\tmean cost\tmean input tok\tmean turns\n"
		for (a in n) printf "%s\t%d\t%.0f%%\t$%.3f\t%.0f\t%.1f\n", a, n[a], 100*p[a]/n[a], c[a]/n[a], i[a]/n[a], t[a]/n[a] }' "$results" | column -t -s $'\t'
echo "(all runs so far in $results)"
