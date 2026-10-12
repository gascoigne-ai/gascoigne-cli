status: later

# `gascoigne learn`: corrections from failed sessions (experimental)

## Goal
Repeated agent mistakes (commands that fail and are retried, reads re-run after truncation) could be
turned into short lines for a local, gitignored instruction file.

## Scope
- Reuse the failures ledger, `probe` and `replay`; dry run prints proposed lines, `-apply` writes a
  marked block to `CLAUDE.local.md` / `AGENTS.md`; reversible like the guide block.
- `experimental: [learn]`. Defer until `probe` data shows enough repeats to be worth a line.

## Acceptance
- Fixture test, deterministic output, block round trip. Value shown on real sessions before graduating.
