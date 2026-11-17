status: todo
kind: chore
commit: refactor(hooks): agent hook replies are typed structs
scope: cmd/gascoigne/agents*.go, cmd/gascoigne/posthook*.go, .harness/architecture/hooks.md

# Hook replies are typed structs

Hook replies have fixed keys known at compile time but are built as `map[string]any`. Use one struct per agent reply. Tool-input fields being rewritten stay a map because unknown keys must be kept as they arrive.

## Model
One struct per agent reply shape; field names and JSON tags equal today's keys. Access pattern: built once, serialized once; no lookup.

## Acceptance
- Pin each agent's current reply first by comparing decoded JSON (not bytes: field order changes).
- `make check` passes.
