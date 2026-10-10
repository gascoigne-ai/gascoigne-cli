status: todo

# Dialect tuning backlog

## Goal
Lift dialects that save little on real output, without losing what agents need.

## Scope
- `doc/help` (curl --help all 2%), `doc/rst` (0% on a README without directives): fold
  option tables (flag + one-line description), cap long examples.
- `vcs/git-diff`/`git show` (16–28%): fold moved blocks and lockfile/generated-file hunks.
- `search/rg` (10% on `rg -n`): group consecutive matches, cap per file by relevance.
- `log/journal` (70%): template unit start/stop pairs ("Starting X…" + "Started X.") into one.
- `jsonx` tables: flatten nested objects to dotted columns (as the cloud engine does) so leaf
  constants hoist for any JSON, then drop the cloud-specific flatten.
- Real fixtures to replace synthetic ones once tools are at hand: terraform/tofu, pulumi, cdk,
  helm diff, kubectl, aws/gcloud/az, mysql, gh (authenticated).

## Acceptance
- Each change: eval case savings up, `keep` strings intact, goldens reviewed, `make check`.
