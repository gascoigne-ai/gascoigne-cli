status: todo
kind: bug
commit: fix(setup): newAgents honors declined agents
scope: cmd/gascoigne/**

# newAgents ignores its settings parameter

`cmd/gascoigne/setup_decisions.go:264` `newAgents(settings prefs)` never reads `settings`, yet its doc says it lists agents "not declined". Verify against callers and `prefs.Declined`.

Test first: a declined agent must not be offered. Fix the filter, or drop the parameter and the doc claim if the behavior is meant. Use `sandbox(t)`.

Model: unchanged
