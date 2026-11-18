status: todo
kind: chore
commit: refactor(optimizer): share table, size and tally helpers
scope: optimizer/**,internal/compress/**

# Dedupe helpers across optimizers

- `table.Limits` (`MaxRows`/`MaxCell`) + `table.Compress(lines, opts)` for `db`, `data`, `k8s`, `cloud`, `table` optimizers. Config keys keep their names.
- One `compress.HumanBytes` for `cloudoptimizer/s3.go:94`, `lsoptimizer/ls.go:141`, `dataoptimizer/notebook.go:122` (unit styles differ: rerun eval, accept only no regression, else keep the styles as a documented option).
- `compress.Tally`, `compress.Indent`, `compress.IsDigit`, `compress.StripHTMLComments`.
- `DedupeRuns` variant for `logoptimizer/log.go:339,456`, `k8soptimizer/apply.go:72`; `payload.Rewrite` in `dataoptimizer/data.go:31`.

Split by bullet when it exceeds one commit. Acceptance: `make check`, `make eval` (no savings regression, `raw ok` never `NO`).
