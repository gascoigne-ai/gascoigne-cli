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
# OpenRouter: AB_ENV_FILE names a dotenv file with OPENROUTER_API_KEY (only
# that line is read, never printed); both arms then go to OpenRouter, "on"
# through gascoigne's proxy on its own server (port 7879, ledger in the
# cache dir) so a running gascoigne server and its stats stay untouched.
# Variants: -a picks arms ("on" alone to compare builds), AB_TAG labels the
# rows (arm "on-<tag>"), so a change to gascoigne itself can be A/B tested
# by running once per build, or a change to the worktree with AB_PREPARE.
# Usage: .harness/scripts/ab.sh [-n] [-r repeats=3] [-m model] [-a "off on"] [task...]
set -euo pipefail
cd "$(dirname "$0")/../.."

dry=0 repeats=3 model=${AB_MODEL:-claude-sonnet-5-5} budget=${AB_BUDGET_USD:-2} arms="off on"
while getopts nr:m:a: o; do
	case $o in
	n) dry=1 ;;
	r) repeats=$OPTARG ;;
	m) model=$OPTARG ;;
	a) arms=$OPTARG ;;
	*) exit 2 ;;
	esac
done
shift $((OPTIND - 1))
tasks=("$@")
[ ${#tasks[@]} -gt 0 ] || mapfile -t tasks < <(find testdata/ab -mindepth 1 -maxdepth 1 -type d -printf '%f\n' | sort)
base=$(cat testdata/ab/BASE)

# OpenRouter mode: provider env for both arms, a proxy config for "on".
key=
if [ -n "${AB_ENV_FILE:-}" ]; then
	key=$(sed -n 's/^OPENROUTER_API_KEY=//p' "$AB_ENV_FILE" | tail -1 | tr -d "\"'\r")
	[ -n "$key" ] || { echo "no OPENROUTER_API_KEY in $AB_ENV_FILE" >&2; exit 2; }
	[ -n "${AB_MODEL:-}" ] || model="~anthropic/claude-sonnet-latest"
fi

go build -o bin/gascoigne ./cmd/gascoigne
gascoigne=$PWD/bin/gascoigne
out=${XDG_CACHE_HOME:-$HOME/.cache}/gascoigne/ab
mkdir -p "$out"
results=$out/ab-results.tsv
tmp=$(mktemp -d)
trap 'git worktree prune; rm -rf "$tmp"' EXIT

openrouter=https://openrouter.ai/api
abaddr=127.0.0.1:7879
if [ -n "$key" ]; then
	cat > "$tmp/ab.yaml" <<YAML
extensions:
  ledger: { path: $out/ab-ledger.db }
receivers:
  httpproxy: { anthropic: $openrouter }
YAML
	trap 'GASCOIGNE_SERVER_ADDR=$abaddr "$gascoigne" server stop >/dev/null 2>&1 || true; git worktree prune; rm -rf "$tmp"' EXIT
fi

# providerEnv sets penv, the variables an arm runs with: none for the
# Anthropic login; for OpenRouter, "off" goes there directly and "on" through
# gascoigne's own proxy, whose upstream is OpenRouter.
penv=()
providerEnv() {
	penv=()
	[ -n "$key" ] || return 0
	if [ "$1" = off ]; then
		penv=(ANTHROPIC_BASE_URL=$openrouter ANTHROPIC_AUTH_TOKEN="$key" ANTHROPIC_API_KEY=)
	else
		penv=(GASCOIGNE_CONFIG="$tmp/ab.yaml" GASCOIGNE_SERVER_ADDR=$abaddr ANTHROPIC_AUTH_TOKEN="$key" ANTHROPIC_API_KEY=)
	fi
}

# clean runs a command without any trace of gascoigne from this shell,
# plus the arm's provider variables (penv).
clean() {
	local unset=(-u ANTHROPIC_BASE_URL)
	for v in $(compgen -e | grep '^GASCOIGNE_' || true); do unset+=(-u "$v"); done
	env "${unset[@]}" "${penv[@]}" "$@"
}

# setup makes a worktree of the base commit with the task's bug planted.
setup() {
	local task=$1 dir=$2
	git worktree add -q --detach "$dir" "$base"
	[ ! -f "testdata/ab/$task/setup.patch" ] || git -C "$dir" apply "$PWD/testdata/ab/$task/setup.patch"
	# AB_PREPARE (a shell command, $gascoigne set) changes the worktree for a
	# variant, e.g. pruned instruction files.
	[ -z "${AB_PREPARE:-}" ] || (cd "$dir" && gascoigne=$gascoigne sh -c "$AB_PREPARE" > /dev/null)
}

check() {
	local task=$1 dir=$2 answer=$3 repo=$PWD
	(cd "$dir" && sh -x "$repo/testdata/ab/$task/check.sh" "$answer")
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
		for arm in $arms; do
			dir=$tmp/$task-$arm
			setup "$task" "$dir"
			cmd=(claude)
			[ $arm = on ] && cmd=("$gascoigne" wrap claude)
			providerEnv $arm
			# The prompt goes on stdin: --allowedTools takes several values and
			# would swallow a prompt argument after it.
			(cd "$dir" && clean "${cmd[@]}" "${flags[@]}" <<< "$prompt" > "$tmp/run.json" 2> "$tmp/run.err") || true
			jq -r '.result // ""' "$tmp/run.json" > "$tmp/answer" 2>/dev/null || : > "$tmp/answer"
			label=$arm${AB_TAG:+-$AB_TAG}
			pass=0
			check "$task" "$dir" "$tmp/answer" > "$out/$task-$label.check" 2>&1 && pass=1
			git -C "$dir" diff --stat >> "$out/$task-$label.check"
			cp "$tmp/run.err" "$out/$task-$label.err" # last run's stderr, for failed runs
			cp "$tmp/run.json" "$out/$task-$label.json"
			[ -s "$tmp/run.json" ] && jq -r --arg t "$task" --arg a "$label" --arg p $pass '[(now|todate), $t, $a, $p, .total_cost_usd, .usage.input_tokens,
				.usage.cache_read_input_tokens, .usage.cache_creation_input_tokens, .usage.output_tokens, .num_turns,
				(.duration_ms/1000|floor)] | @tsv' "$tmp/run.json" >> "$results" ||
				printf '%s\t%s\t%s\t0\t\t\t\t\t\t\t\n' "$(date -Is)" "$task" "$label" >> "$results"
			echo "$task $label #$rep: pass=$pass"
			git worktree remove --force "$dir"
		done
	done
done

echo
awk -F'\t' 'NR > 1 { n[$3]++; p[$3] += $4; c[$3] += $5; i[$3] += $6 + $7 + $8; t[$3] += $10 }
	END { printf "arm\truns\tpass\tmean cost\tmean input tok\tmean turns\n"
		for (a in n) printf "%s\t%d\t%.0f%%\t$%.3f\t%.0f\t%.1f\n", a, n[a], 100*p[a]/n[a], c[a]/n[a], i[a]/n[a], t[a]/n[a] }' "$results" | column -t -s $'\t'
echo "(all runs so far in $results)"
