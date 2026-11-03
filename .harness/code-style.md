# Code style

Adds to the shared `common/code-style.md` (readability first, shape, errors, comments, docs, tests).

## Go specifics
- gofmt is law (a hook runs it after every edit).
- Modern Go where it shortens code: `slices`, `maps`, `cmp.Or`, `min`/`max`,
  `for range n`, range-over-func iterators, `sync.WaitGroup.Go`, `t.Context()`.
- Errors: `fmt.Errorf("load %s: %w", path, err)`. Sentinel errors as `ErrX`.
- Logging: `log/slog` from `component.Settings.Logger`. Debug for flow, Warn for degraded.
- Accept interfaces, return structs. Generics: see `component.NewFactory`.
- Config flows through factories.
- Doc comment on every exported identifier that isn't self-evident.

## Tests
- Same package (`package foo`), table-driven, `t.Run` for named cases.
- Fakes: see `service_test.go`.
- Temp dirs via `t.TempDir()`, ledger via `ledger.EnvPath` / `Config.Path`.
- Tests that touch configs or state use `sandbox(t)` (`cmd/gascoigne`): it redirects `HOME` and the
  `XDG_*` dirs.
