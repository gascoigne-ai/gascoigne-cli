status: todo
kind: chore
commit: refactor(optimizer): fold single-consumer compress libs into their optimizer
scope: internal/compress/drain/**, internal/compress/trace/**, internal/compress/pkglist/**, internal/compress/tree/**, optimizer/logoptimizer/**, optimizer/traceoptimizer/**, optimizer/pkgoptimizer/**, .harness/architecture/families.md

# Fold single-consumer compress libs

Each of these packages has exactly one importer, so it is not a shared primitive. `gitoptimizer` and
`logoptimizer` already own their engines; this matches that practice.

| Package | LOC | Importer | Target |
|---|---|---|---|
| `internal/compress/drain` | 166 | `logoptimizer` | `optimizer/logoptimizer/drain.go`, unexport |
| `internal/compress/trace` | 265 | `traceoptimizer` | into `traceoptimizer` (now a 92-line shell) |
| `internal/compress/pkglist` | 107 | `pkgoptimizer` | into `pkgoptimizer` |
| `internal/compress/tree` | 304 | `pkgoptimizer` | into `pkgoptimizer` |

Steps:
1. Re-count importers with grep first; skip any package that gained a second one.
2. One package per commit step: move files and tests, unexport what no one else uses, fix imports.
3. Update `architecture/families.md` (line 41 names `drain`; re-grep for `trace`, `tree`, `pkglist`) and
   `docs/` mentions.

Not in scope: `probe` and `symbols` (one importer outside the optimizers, see backlog), `compose`, `watch`.

Acceptance: four fewer packages, tests moved not edited, `make check` and `make eval` pass with
identical numbers.
