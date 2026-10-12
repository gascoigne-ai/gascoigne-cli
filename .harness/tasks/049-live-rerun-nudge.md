status: todo

# Rerun nudge in the hint

## Goal
`learn` finds, after the fact, that agents re-run a command whose output was shortened instead of
reading the original with `gascoigne raw <id> -grep`. Say it at the moment it happens.

## Scope
- Core's hint is fixed text; the nudge belongs to the `rerun` optimizer's marker, which already
  knows the earlier runs of the same key in the session. When a key was shortened two or more times
  in the session, the marker line names the first raw id: `same command as <id>: gascoigne raw <id> -grep PATTERN`.
- Keep the text short (hints cost tokens). No change to `payload/` or `internal/service` (core needs
  approval); if the marker cannot carry it, stop and say so.
- Optimizers stay deterministic: the text depends only on ledger rows of this session.

## Acceptance
- Table tests: first run no nudge, second run nudge with the right id, other session none.
- `make eval`: savings do not regress, `raw ok` never `NO`.
- `learn` counts `rerun` habits on a fixture ledger. No paid A/B: compare the rerun count in real
  ledgers one week before and after, noted in `backlog.md`.
