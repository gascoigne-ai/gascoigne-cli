status: todo
kind: chore
commit: feat(truncate): lower the default token budget after replay
scope: internal/config/**, optimizer/truncate/**, .harness/backlog.md, .harness/decisions.md

# Stronger truncate default (measure first)

The truncate budget is 10000 tokens. In the live ledger, output above 4000 tokens is 757k in 257 events;
about 340k of it is truncate-eligible and not noise (167k is `make eval`). Estimated gain from a 5000
budget: 100-200k per 7 weeks, minus rerequests. Medium risk: the agent may retrieve more.

Steps:
1. Run `gascoigne replay -sessions 25 -set truncate.max_tokens=5000` and the base run; compare saved
   tokens and rerequests (`Ledger.Impact`). Record both numbers in `.harness/decisions.md`.
2. Only if the net gain is over 50k per 7 weeks, change the default and add a test for the new value.
3. Otherwise record the result under "Tried and dropped" in `.harness/backlog.md` with the numbers.

Acceptance: decision recorded with numbers either way; if changed, `make check && make eval` pass and
`testdata/eval/baseline.tsv` is re-pinned. After release, watch `gain -by reason` for two weeks.
