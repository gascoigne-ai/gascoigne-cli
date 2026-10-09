status: todo

# richer gain report

## Goal
`gascoigne gain` answers "where do savings and failures come from".

## Scope
- Flags: `-by route|key|day` (default route), `-since 7d`, `-top N`.
- Per-key view shows failure count next to savings.
- New gorm queries in `extension/ledger/report.go`; add indexes via gorm tags in `models.go`
  if needed (AutoMigrate creates them).

## Acceptance
- Ledger tests for each grouping with seeded events.
- `make check` green.
