status: todo
kind: chore
commit: chore(cost): refresh provider prices
scope: internal/cost/**
not_before: 2026-12-01

# Re-read provider prices on 2026-12-01

Gemini 4 Argon and Carbon move from 2 / 10 / 0.10 to 4 / 20 / 0.20 on 2026-12-01 (see the comment
above the rows in `internal/cost/cost.go`), and the gpt-5.6-sol promotion ends 2026-11-30.
Re-read the three `PriceSources`, update the rows and the comment, bump `PricesChecked`.

Test first: the existing lookup table plus a price assertion for the changed rows.

Acceptance: `make check` passes.
