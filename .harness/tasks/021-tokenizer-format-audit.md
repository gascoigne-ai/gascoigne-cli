status: todo

# Tokenizer-aware formatting audit

## Goal
Make sure gascoigne's own markers and layouts are cheap in each tokenizer family.

## Scope
- Measure with `gascoigne tokens`: `…` markers, `§1` path ids, `_cols/_rows`, `|` tables,
  `N×` counts, hint line; compare alternatives.
- Change only where `make eval` out tokens drop; record results in recommendations.md.

## Acceptance
- `make check` + `make eval` green; baseline updated with the gains.
