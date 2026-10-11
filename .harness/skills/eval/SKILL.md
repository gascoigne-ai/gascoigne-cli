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
   count as kept, sampling and caps as lost), `check` = all assertions of the case held.
3. Per case assertions: `expect_type`, `keep` (substrings that must survive), `min_saved` (%),
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
