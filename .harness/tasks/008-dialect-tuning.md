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

## Open
- `log/journal`: drop "Starting X…" once "Started/Finished X." follows (patch exists). Measured
  70.0→68.5%: not bigger output, but the `max_lines` cap then keeps more distinct events (omitted
  304→220). Needs a decision: accept as an information gain, or move the cap to a token budget.
- `doc/help` (curl --help all 2%): every line is one option + description; only alignment
  padding is left, which a real tokenizer merges (see recommendations). Worth it only with a
  `query` (keep matching options), e.g. via exec `-attr query=…`.
- `doc/rst` (0% on a README without directives): needs a fixture with directives/badges.
- `search/rg -n` (14%): remaining lines are distinct matches already grouped and capped; more
  savings would drop matches. Leave unless a relevance cap per file is wanted.
- `fs/code`: doc comments separated from their declaration by a blank line (Ruby rdoc) are
  dropped whole; keep their first line. Lowers savings slightly by design (readability fix).
- Real fixtures to replace synthetic ones once tools are at hand: terraform/tofu, pulumi, cdk,
  helm diff, kubectl, aws/gcloud/az, mysql, gh (authenticated).

## Acceptance
- Each change: eval case savings up, `keep` strings intact, goldens reviewed, `make check`.
