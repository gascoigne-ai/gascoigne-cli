---
name: add-component
description: Add a new gascoigne component (receiver, filter, optimizer, exporter, extension). Use when implementing a new optimizer, filter, receiver, exporter or extension.
---

# Add component

Read `.harness/architecture.md` first. Never edit core (`component/`, `payload/`, `tokens/`, `internal/{config,router,service}`)
to make a component work — if you think you must, stop and ask the user.

## Steps
1. Package `<kind>/<name><kind>/` at repo root — never under `internal/`; import only
   `component`, `payload`, `tokens` and stdlib/approved deps (`make imports` enforces) (e.g. `optimizer/jsoncompactoptimizer/`).
2. `factory.go`:
   ```go
   const typ = "jsoncompact"

   func NewFactory() component.Factory {
   	return component.NewFactory(component.KindOptimizer, typ,
   		func() *Config { return &Config{ /* defaults */ } },
   		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
   			return &Optimizer{cfg: cfg}, nil
   		})
   }
   ```
3. `config.go`: `Config` with `yaml:"snake_case"` tags, documented fields, `Validate() error`
   when there are constraints. Defaults live in the factory, not in Validate.
4. `<name>.go`: package doc comment; struct embeds `component.Nop` unless it needs lifecycle;
   implement the kind's method (`Optimize`, `Filter`, `Export`, `Run`).
   - Need a shared service? Look it up in `Start` via `host.Extension(id)`; id comes from config.
   - Optimizers: select blocks by content type with `p.Rewrite(fn, detect.X, ...)`; never
     touch other blocks. Deterministic; return an error instead of half-broken output (core
     falls back to the original and records the failure). Reuse `compress/` primitives.
   - New content type: add constant, cmd mapping/sniffer and tests in `detect/` first.
5. `<name>_test.go`: table-driven, real-shaped inputs.
6. Register: one line in `internal/registry/registry.go` under the right kind comment.
7. Optimizers: add to `optimizers:` and the router `default:` chain in `configs/default.yaml`.
   Filters: add to the pipelines' filter lists.
8. Optimizers: fixtures in `testdata/eval/<tool>/` + cases (`expect_type`, `keep`, `min_saved`)
   — see skill `eval`.
9. Run skill `verify`, then `eval`.
