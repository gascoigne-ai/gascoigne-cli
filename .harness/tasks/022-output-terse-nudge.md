status: todo

# Guide: terse output nudge

## Goal
Output tokens cost ~5× input; the agent guide may cut narration.

## Scope
- One line in `guide.go` asking for brief status text.
- A/B with `.harness/scripts/ab.sh` (task success and output tokens); keep only if success
  does not drop and output tokens fall.

## Acceptance
- ab.sh results in the commit message; `make check` green.
