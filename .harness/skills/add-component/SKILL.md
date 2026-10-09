---
name: add-component
description: Add a new gascoigne component (receiver, filter, optimizer, exporter, extension). Use when implementing a new optimizer, filter, receiver, exporter or extension.
---

# Add component

Read `.harness/architecture.md` first. Never edit core (`internal/{component,config,router,service,payload}`)
to make a component work — if you think you must, stop and ask the user.

## Steps
1. Package `internal/<kind>/<name><kind>/` (e.g. `internal/optimizer/jsoncompactoptimizer/`).
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
   - Optimizers: mutate `p` in place, deterministic, return error instead of producing
     half-broken output (core falls back to the original).
5. `<name>_test.go`: table-driven, real-shaped inputs.
6. Register: one line in `internal/registry/registry.go` under the right kind comment.
7. Optimizers/filters: wire into `configs/default.yaml` (route or pipeline filter list).
8. Optimizers: add fixture to `testdata/eval/` + case in `cases.yaml`
   (no secrets, no real home paths).
9. Run skill `verify`, then `eval`.
