#!/usr/bin/env bash
# Autonomous agent loop: works through .harness/tasks/*.md marked `status: todo`.
# Each task: headless claude → make check + make eval → delete task + commit, or stop.
#
# usage: loop.sh [--dry-run] [--once]
# env:   MAX_TURNS (default 50), CLAUDE (default claude)
set -euo pipefail
shopt -s nullglob
cd "$(dirname "$0")/../.."

dry_run=false once=false
for arg in "$@"; do
	case $arg in
	--dry-run) dry_run=true ;;
	--once) once=true ;;
	*) echo "usage: $0 [--dry-run] [--once]" >&2; exit 2 ;;
	esac
done

max_turns=${MAX_TURNS:-50}
claude_bin=${CLAUDE:-claude}
logs=.harness/logs
mkdir -p "$logs"

pending() {
	for f in .harness/tasks/*.md; do
		[[ $(head -1 "$f") == "status: todo" ]] && echo "$f"
	done
}

prompt() {
	cat <<PROMPT
You are working autonomously on gascoigne. Follow .harness/rules.md and .harness/code-style.md
strictly; read .harness/architecture.md (index) and the .harness/architecture/ file for your area before changing code.

Complete this task fully, then run the verify skill (make check, and make eval if optimizers,
filters, routes or default config changed). Do not commit and do not edit the task file;
the harness does both. If the task is blocked or needs a core change, stop and explain why.

Task file: $1
---
$(tail -n +2 "$1")
PROMPT
}

tasks=$(pending)
if [[ -z $tasks ]]; then
	echo "no pending tasks"
	exit 0
fi

if $dry_run; then
	echo "pending tasks:"
	printf '  %s\n' $tasks
	exit 0
fi

if [[ -n $(git status --porcelain) ]]; then
	echo "working tree dirty; commit or stash first" >&2
	exit 1
fi

for task in $tasks; do
	name=$(basename "$task" .md)
	log="$logs/$name-$(date +%Y%m%d-%H%M%S).log"
	echo "==> $name (log: $log)"

	if ! "$claude_bin" -p "$(prompt "$task")" \
		--permission-mode acceptEdits --max-turns "$max_turns" >"$log" 2>&1; then
		echo "claude failed on $name; see $log" >&2
		exit 1
	fi
	if ! { make check && make eval; } >>"$log" 2>&1; then
		echo "verification failed on $name; see $log (changes left in working tree)" >&2
		exit 1
	fi

	title=$(sed -n 's/^# //p' "$task" | head -1)
	git rm -q "$task" # done tasks live in git history
	git add -A
	git commit -q -m "$title" -m "Task: $task"
	echo "    done: $(git log -1 --format=%h)"

	$once && break
done
