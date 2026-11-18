status: todo
kind: chore
commit: refactor(cmd): share json state, hooks and transcript helpers
scope: cmd/gascoigne/**,inproc/**,internal/mcptool/**
depends_on: 016-cmd-swallowed-errors

# Dedupe helpers in cmd

- State files: use `readJSONFile` (`mcpconfig.go:94`) and `saveJSON` (`fileutil.go:14`) in `prefs.go`, `release.go`, `installs.go`, `rules.go`, `setup_token.go`.
- `cursorHooks(exe)` for `install.go:225` and `agents.go:317`.
- `scanTranscript(path, func(line))` for `continuity.go`, `context_session.go`, `replay.go`, `prompts.go` (buffer caps differ: pick the larger).
- `removeBlock(file, start, end)` for `guide.go:141` and `learn.go:113`.
- `sdkPipeline` -> `inproc.Pipeline` (`sdk.go:24`); one hidden-prefix predicate for `dashboard.go:243` and `inproc/inproc.go:31`; one exported `shellQuote` (`hook.go:289`, `mcptool/attrs.go:73`).

Depends on 016 (same files). Acceptance: `make check`.
