---
name: eval
description: Measure gascoigne token savings on fixtures. Use when changing optimizers, routes or filters, or when asked about savings/benchmarks.
---

# Eval

1. `make eval` — builds, runs `testdata/eval/cases.yaml` through the `cli` pipeline with a
   throwaway ledger (`GASCOIGNE_LEDGER` temp path; real ledger untouched).
2. Read the table: `route` shows which optimizer chain ran; `-` means a filter bypassed it
   (e.g. `minsize`). `raw ok = yes` means the original came back byte-exact.
3. Any `NO` or non-zero exit is a blocker: information would be lost.
4. Savings: report before/after `saved %` per case you affected. A drop needs a reason.
5. New input shape? Add a fixture + case. Fixtures must be realistic and sanitized.

Custom runs: `bin/gascoigne eval -config my.yaml -cases path/cases.yaml -pipeline cli`.
