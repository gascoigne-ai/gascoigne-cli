# Code style

**Readability is number one, over everything** — over cleverness, performance, DRY, and
consistency with bad code. If a reader has to pause, rewrite it.

## Shape
- Concise syntax. Short, clear names: `p` for payload in small scopes, full words in wide ones.
- Early returns; keep the happy path flat and left-aligned. Max ~2 levels of nesting.
- Small functions doing one thing. A function that needs a section comment wants splitting.
- Fewer lines beat abstractions: three similar lines are fine; extract on the third real copy.

## Go specifics
- gofmt is law (a hook runs it after every edit).
- Modern Go where it shortens code: `slices`, `maps`, `cmp.Or`, `min`/`max`,
  `for range n`, range-over-func iterators, `sync.WaitGroup.Go`, `t.Context()`.
- Errors: wrap with context, `fmt.Errorf("load %s: %w", path, err)`. Sentinel errors as `ErrX`.
  Don't log and return the same error.
- Logging: `log/slog` from `component.Settings.Logger`. Debug for flow, Warn for degraded.
- Interfaces: only the core component contracts, or once a 2nd implementation exists.
  Accept interfaces, return structs.
- Generics only when they remove real duplication (see `component.NewFactory`).
- No global mutable state; config flows through factories.
- Context first param on anything doing I/O.

## Comments
- Say why, never what. Doc comment on every exported identifier that isn't self-evident.
- Package doc comment in the main file of each package.
- No commented-out code, no TODO without a task file in `.harness/tasks/` or an Open item in
  `recommendations.md`.

## Docs
- Diagrams and flows are Mermaid (a fenced `mermaid` block in the Markdown), not ASCII art or
  images: they render on GitHub, diff as text and need no tool to regenerate. Short labels, quoted
  when they hold punctuation, node ids that are not Mermaid keywords (`end`, `graph`, `in`).
  Check a new diagram with Mermaid's parser (`mermaid.parse`, under jsdom in node) before committing.
- Screenshots stay images (`docs/images/`); tables stay tables.

## Tests
- Same package (`package foo`), table-driven, `t.Run` for named cases.
- Fakes over mocks: tiny structs implementing the contract (see `service_test.go`).
- Temp dirs via `t.TempDir()`, ledger via `ledger.EnvPath` / `Config.Path`.
- Tests that touch configs or state use `sandbox(t)` (`cmd/gascoigne`): it redirects `HOME` and the
  `XDG_*` dirs. Setting only `HOME` is not enough when `XDG_CONFIG_HOME` is set: a test once overwrote
  the real user config.
- Test behavior, not implementation details.
