---
name: eval
description: Measure gascoigne token savings on fixtures. Use when changing optimizers, detection, routes or filters, or when asked about savings/benchmarks.
---

# Eval

1. `make eval` — builds, runs `testdata/eval/cases.yaml` through the `cli` pipeline with a
   throwaway ledger (`GASCOIGNE_LEDGER` temp path; real ledger untouched).
2. Table: `type` = block types after optimization, `route` = chain that ran (`-` = a filter
   bypassed it, e.g. `minsize`), `lost` = error lines/artifacts (paths, URLs, hashes) of the
   original no longer readable in the output (`compress/probe`; regrouping and path legends
   count as kept, sampling and caps as lost), `ms` = wall time of one optimization (not in the
   baseline; look into outliers), `check` = all assertions of the case held.
3. Per case assertions: `expect_type`, `keep` (substrings that must survive), `min_saved` (%),
   `unchanged` (must pass through), `lossless` (may shrink, no error line or artifact lost),
   output never larger than input,
   golden output in `testdata/eval/golden/<name>.out`, original retrievable byte-exact,
   out tokens and lost counts equal to `testdata/eval/baseline.tsv` (the failure says which
   way they moved: more tokens or more lost facts is a regression to justify).
   Any failure is a blocker. Never lower `min_saved` or drop a `keep` to make a run pass
   without telling the user why.
4. Look at outputs: `bin/gascoigne eval -show <name-substring>`. After an intended output
   change, review the golden diff, then `bin/gascoigne eval -update` (writes goldens only for
   cases whose output changes, rewrites `baseline.tsv`) and commit both with the code; the
   baseline diff is the savings/retention record of the change.
5. New input shape: realistic, sanitized fixture (no secrets, emails, home paths) + case with
   `keep` for what an agent must still see, `min_saved` a bit under the measured value, and a
   detection-only variant (no attrs) when the type should be sniffable.

Custom runs: `bin/gascoigne eval -config my.yaml -cases path/cases.yaml -pipeline cli`.

Real sessions: `bin/gascoigne replay [-sessions N] [-json]` rebuilds Claude Code transcripts'
requests and sends them through the LLM API proxy (stand-in upstream, throwaway ledger):
savings per tool, message input cost before/after (cache reads for the prefix the previous
request sent identically, writes for the rest; model from the transcript), and prompt-cache stability (messages sent again must stay byte-identical;
anything below 100% is a bug). Bash results in transcripts were already shortened by the hook,
so its column is the proxy's extra. Compare with another ref: `.harness/scripts/compare.sh
[REF=master] [N=10]` builds REF in a temp worktree, replays the same N newest transcripts with
both builds (`replay -compare old.json files…`) and shows the `baseline.tsv` diff (fixtures).

Experimental features (opt-in, extra installs): `make eval-experimental` runs
`testdata/eval-experimental/cases.yaml` with them enabled; skipped by `make eval` and `make check`.

Live drift: `make eval-live` runs `testdata/eval/live.yaml` (`run:` cases: real commands in the
repo, output varies, so no golden/baseline; missing programs skip). A wrong type or a floor
missed there while the fixture case passes means the tool's output changed shape: refresh the
fixture from the live output.

Accuracy against references: a case's `reference:` file is text the output must hold (word
recall/precision printed, `min_recall` floor). `.harness/scripts/html-bench.sh` runs the public
article extraction benchmark (181 pages, cloned into the cache dir, never the repo) through the
cli pipeline as fetched pages: recall/precision of the article text and savings.

Task success (costs API money, never in `make check`): `.harness/scripts/ab.sh [-n] [-r 3] [-m
model] [task…]` runs each `testdata/ab/<task>` (prompt.txt, setup.patch planting a bug on
`testdata/ab/BASE`, check.sh judging the result) with `claude -p` off (no user settings, MCP or
gascoigne env) and on (`gascoigne wrap claude`), then prints pass rate, cost, input tokens and
turns per arm. `-n` checks the tasks for free: each must apply and fail its check unsolved. A
new task needs a check that passes on the solved state (try it on the base commit).
