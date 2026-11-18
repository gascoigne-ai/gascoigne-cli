status: blocked
kind: feature
commit: refactor(component): shared optimizer factory helper
scope: component/**,optimizer/**,.harness/architecture/**
depends_on: 034-merge-web-optimizers

# One factory helper for optimizers

19 of 29 `optimizer/*/factory.go` are an identical 21 lines (`typ`, `family`, `NewFactory` returning `&Optimizer{cfg: cfg}`). Add `component.NewOptimizerFactory(typ, family, defaults, build)`.

CORE CONTRACT CHANGE (`component/`): needs explicit user approval before editing; `rules.md` treats it as an API change. Skip if the helper saves fewer lines than it adds once the 10 non-identical factories are counted.

## Model
No new state: a constructor helper over the existing `component.NewFactory`. Rejected: a registry-wide generic repository.

Acceptance: `make check`, `make eval`.
