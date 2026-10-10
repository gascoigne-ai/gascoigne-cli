status: todo

# Dialect tuning backlog

## Goal
Lift dialects that save little on real output, without losing what agents need.

## Done
- `vcs/git-diff`: in-line edits as word diffs, lockfile/generated hunks folded (git/diff 28→31.5%,
  real go.sum commit 42%). Moved blocks: removed runs re-added verbatim become one `moved to` line (real fixture from this repo: 2054→1832).
- `vcs/git-log`: `--stat`/`--name-only`/`--name-status` lists were dropped (bug); kept, stat bars
  reduced to their signs.
- `jsonx` tables: items with nested objects also tried flattened to dotted columns, smaller wins;
  cloud uses the shared `jsonx.Flatten` (github API 63→74%, github-repos 83.5→86%).

- `log/journal`/`log/syslog`: "Starting X…"/"Stopping X…" dropped once the same process reports
  X started, finished or stopped. 69.6→67.6%: same 300-line cap, 84 more distinct events kept.

- `doc/help` with a `query`: only matching option entries stay (curl --help all 0 → 84%).

- `doc/rst`: real README with badges (vdf, 30.7%): badge-only substitution rows dropped with their
  image definitions; link targets (`.. _name: url`) kept, they were dropped as comments.

- `search`: with a `query`, each file keeps the `max_per_file` lines matching it best (BM25),
  in file order, instead of the first ones.

- `fs/code`: a comment run one blank line above code at its indent counts as documentation and
  keeps its summary line (Ruby rdoc 56.2%, was more; file headers before any code still go).

## Open
- Real fixtures to replace synthetic ones once tools are at hand: terraform/tofu, pulumi, cdk,
  helm diff, kubectl, aws/gcloud/az, mysql, gh (authenticated).

## Acceptance
- Each change: eval case savings up, `keep` strings intact, goldens reviewed, `make check`.
