status: todo

# richer gain report

## Goal
`gascoigne gain` answers "where do savings and failures come from".

## Scope
- Flags: `-by route|key|day` (default route), `-since 7d`, `-top N`.
- Per-key view shows failure count next to savings.
- New ledger queries in `extension/ledger/report.go`; add index migration if needed
  (new `migrations/002_*.sql`, never edit 001).

## Acceptance
- Ledger tests for each grouping with seeded events.
- `make check` green.
