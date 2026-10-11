#!/usr/bin/env bash
# Compares this checkout with another git ref on the same real sessions:
# builds REF in a temporary worktree, replays the newest N Claude Code
# transcripts with both builds, prints the changes. Fixture savings and
# retention per case are compared by `git diff REF -- testdata/eval/baseline.tsv`.
# Usage: .harness/scripts/compare.sh [REF=master] [N=10]
set -euo pipefail
cd "$(dirname "$0")/../.."
ref=${1:-master}
n=${2:-10}

tmp=$(mktemp -d)
trap 'git worktree remove --force "$tmp/ref" >/dev/null 2>&1 || true; rm -rf "$tmp"' EXIT
git worktree add --detach "$tmp/ref" "$ref" >/dev/null
(cd "$tmp/ref" && go build -o "$tmp/old" ./cmd/gascoigne)
go build -o bin/gascoigne ./cmd/gascoigne

mapfile -t files < <(find "${CLAUDE_PROJECTS:-$HOME/.claude/projects}" -name '*.jsonl' -printf '%T@ %p\n' | sort -rn | head -n "$n" | cut -d' ' -f2-)
"$tmp/old" replay -json "${files[@]}" > "$tmp/old.json"
bin/gascoigne replay -compare "$tmp/old.json" "${files[@]}"
echo
git diff --stat "$ref" -- testdata/eval/baseline.tsv
