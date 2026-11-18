status: todo
kind: bug
commit: fix(hook): run only core optimizers on mixed-family output
scope: filter/detectfilter/**, receiver/hookreceiver/**, testdata/eval/**, .harness/architecture/flow.md

# Mixed-blob guard

When one Bash line produces output from two or more commands of different families, the blob is typed
from one command and a family engine can reorder or fold lines across parts. Seen: a three-part script
was typed `test/go`, and an `ok` line from part 1 was folded into a summary placed after part 3.

Change: when the command has two or more output-producing segments of different families, run only the
core optimizers (`dense`, `paths`, `truncate`) and skip family engines. Do after task 011 lands, because
011 removes most such blobs; this guard covers the rest (loops, heredocs, subshells).

Test first: a case with `go test` output followed by `git status` output; assert no line from one part
appears inside the other part's summary.

Acceptance: new eval case passes with `raw ok` never `NO`; `make check && make eval` pass. Savings are
expected near zero; this is a correctness fix.

Model: unchanged
