status: blocked
kind: feature
commit: refactor(web): one web optimizer with a dialect table
scope: optimizer/**,internal/registry/**,configs/**,testdata/eval/**,.harness/architecture/**
depends_on: 032-modern-go-sweep

# One engine for the web family

`families.md` says one engine per family, but `web` has four packages (`htmloptimizer`, `httpoptimizer`, `ariaoptimizer`, `websearchoptimizer`, each `family = "web"`). Merge into `weboptimizer` with a dialect table (html, http, aria, search). Fold `jsonoptimizer` (27 lines) into `dataoptimizer` as json/ndjson rows. Decide for `lsoptimizer` + `catoptimizer` (fs): merge or document the exception in `families.md`.

User approval needed before starting (config keys and registry change). Keep old config keys working or write a migration note. Update `registry`, `configs/default.yaml`, eval cases, `architecture/families.md` in the same commit.

## Model
Write the design note with the dialect table structure and the per-dialect config; access pattern: lookup by detected type, ~5 rows; rejected: one package per dialect; revisit when rows pass ~15.

Acceptance: `make check`, `make eval` (no savings regression).
