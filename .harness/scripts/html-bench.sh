#!/usr/bin/env bash
# Web page accuracy: runs the article extraction benchmark (181 pages with
# their article text, MIT, github.com/scrapinghub/article-extraction-benchmark)
# through the cli pipeline as fetched pages. Reports word recall/precision of
# each article in the output and the savings. The dataset is cloned into the
# user cache dir, never into the repository.
# Usage: .harness/scripts/html-bench.sh [extra gascoigne eval flags]
set -euo pipefail
cd "$(dirname "$0")/../.."

data=${XDG_CACHE_HOME:-$HOME/.cache}/gascoigne/bench/article-extraction
if [ ! -d "$data/repo" ]; then
	mkdir -p "$data"
	git clone -q --depth 1 https://github.com/scrapinghub/article-extraction-benchmark "$data/repo"
fi
mkdir -p "$data/html" "$data/ref"
cases=$data/cases.yaml
: > "$cases"
for id in $(jq -r 'keys[]' "$data/repo/ground-truth.json"); do
	[ -f "$data/html/$id.html" ] || gunzip -c "$data/repo/html/$id.html.gz" > "$data/html/$id.html"
	jq -r --arg id "$id" '.[$id].articleBody' "$data/repo/ground-truth.json" > "$data/ref/$id.txt"
	url=$(jq -r --arg id "$id" '.[$id].url' "$data/repo/ground-truth.json")
	printf -- '- name: %s\n  file: html/%s.html\n  attrs: { url: %s, mime: text/html }\n  reference: ref/%s.txt\n' \
		"$id" "$id" "$(jq -n --arg u "$url" '$u')" "$id" >> "$cases"
done

go build -o bin/gascoigne ./cmd/gascoigne
exec bin/gascoigne eval -cases "$cases" "$@"
