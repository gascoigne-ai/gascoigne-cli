status: todo

# Guide level: off, short, full

## Goal
Some users want less injected text. Offer `off|short|full` (default full), as the awareness level of
comparable tools does. Short keeps only what an agent cannot guess: the `§` path ids and the
`gascoigne raw` / retrieve hint line.

## Scope
- Source: env `GASCOIGNE_GUIDE`, else the `guide` key of the small state file
  `$XDG_STATE_HOME/gascoigne/prefs.json` (new `prefs.go`, written atomically; shared with tasks 033-035).
  Not `config.yaml`: a new key there means a change in `internal/config` (core, needs approval).
- `off`: claude SessionStart still starts the proxy/server but adds no context; AGENTS.md blocks
  and the cursor rule are removed on the next install.
- `short` text lives next to `guide()`; both levels in `guide_test.go`.

## Acceptance
- Tests per level for `guide`, `withGuide`, `addGuide`.
- Architecture §Agents documents the level and where it is read.
