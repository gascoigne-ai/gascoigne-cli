# Extending

Most new support is a **dialect**: a new tool in a family gascoigne already has. A new
**optimizer** is only for a new family. Other component kinds (receivers, filters, exporters,
extensions) follow the same package shape.

Never change the core (`component/`, `payload/`, `tokens/`, `internal/{config,router,service}`)
to make a component work; the contracts are APIs every component depends on.

## Add a dialect

Example: a new test runner, `mytest`.

1. **Detection.** Add a `Spec` in `internal/detect/test.go`:

   ```go
   Spec{Type: MyTest, Cmds: []string{"mytest"}, Priority: 30, Sniff: anyLine(`^MYTEST v\d`)}
   ```

   and the type constant (`MyTest = "test/mytest"`). Use `Refines` when it specializes a generic
   file or MIME type, `CmdFunc` when a flag value decides it.
2. **Rules.** Teach the family's engine or library what to keep and drop: for tests,
   `internal/compress/testreport`; for builds, `internal/compress/diag`; for package managers,
   `internal/compress/pkgnoise`; for YAML, `internal/compress/yamlx`; for tables, `internal/compress/table`.
3. **Matrix.** Add a command example and a content example in `internal/detect/matrix_test.go`.
4. **Fixture.** Save realistic, sanitized output in `testdata/eval/<tool>/` (no secrets,
   emails or home paths) and add a case to `testdata/eval/cases.yaml`:

   ```yaml
   - file: test/mytest-fail.txt
     attrs: { cmd: mytest ./... }
     expect_type: test/mytest
     keep: ["FAIL: checkout total", "expected 0, got -4"]   # what an agent must still see
     min_saved: 50                                          # a bit under the measured value
   - { name: detect/mytest, file: test/mytest-fail.txt, expect_type: test/mytest }  # sniffing alone
   ```
5. Run `make check` and `make eval`, review the output with `bin/gascoigne eval -show mytest`,
   then `bin/gascoigne eval -update` to write its golden file.

## Add an optimizer (a new family)

```
optimizer/<name>optimizer/
  factory.go     NewFactory() via component.NewFactory, const typ, component.WithFamily
  config.go      Config with yaml tags, documented fields, Validate() for constraints
  <name>.go      the engine (package doc comment here)
  <name>_test.go table-driven, real-shaped inputs
```

```go
// factory.go
const typ = "mything"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindOptimizer, typ,
		func() *Config { return &Config{MaxRows: 30} }, // defaults live here
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return &Optimizer{cfg: cfg}, nil
		}, component.WithFamily("mything"))
}
```

```go
// mything.go
func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		return o.shrink(b.Text), nil
	}, detect.MyThing)
}
```

Then:

1. One line in `internal/registry/registry.go`.
2. An entry under `optimizers:` and in `router.default` in `configs/default.yaml`.
3. Eval fixtures and cases, as for a dialect.

Rules for optimizers:

- **Deterministic and idempotent**: same input, same output; running twice changes nothing
  more.
- **Only your blocks**: select them with `p.Rewrite(fn, types…)`, or `p.RewriteFamily(fn, family)` for a whole family.
- **Errors, not half-broken output**: the core falls back to the original and records it.
- **Explain deliberate no-ops**: set `payload.AttrSkip` (`skip_reason`) to a factual reason,
  e.g. `mything: 120 rows < max_rows 200`.
- **Reuse libraries**: `internal/compress/`, `internal/compress/sample`, `internal/compress/table`, `internal/compress/jsonx`. An
  engine needing another family's logic uses its public library, never its package.
- **Keep marker text short**; it costs tokens too.

## Other components

| Kind | Implement | Typical use |
|---|---|---|
| receiver | `Run(ctx)`; push payloads through `Settings.Pipeline` | a new input channel |
| filter | `Filter(ctx, p) (component.Decision, error)` | a new reason to skip optimization |
| exporter | `Export(ctx, p) error` | send results somewhere else |
| extension | lifecycle only | a shared service, found by others via `host.Extension(id)` |

Filters go into the pipelines' `filters:` lists; receivers and exporters into a pipeline.
Component packages may import only public packages (`component`, `payload`, `tokens`,
`detect`, `compress`, `mcptool`) and approved dependencies, never `internal/` and never
another component.
