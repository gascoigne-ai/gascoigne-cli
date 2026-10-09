status: todo

# jsoncompact optimizer

## Goal
Cut tokens in large JSON payloads (API responses, `go list -json`, tool results).

## Scope
- Package `optimizer/jsoncompactoptimizer`. Stdlib `encoding/json` only.
- Minify; drop `null`, `""`, `[]`, `{}` values (config `drop_empty`, default true).
- Arrays of objects with identical key sets → compact table form
  `{"_cols":[...],"_rows":[[...],...]}` (config `tabulate_min_rows`, default 3).
- Concatenated JSON values (like `go list -json` output) handled value by value.
- Invalid JSON → return error (core falls back to original and records the failure).
- Replace `passthrough/big` in the `big-json` route.

## Acceptance
- Eval: `go-list-m.json` saves ≥ 30%, `raw ok = yes`.
- Deterministic output (map key order sorted) — test runs it twice and compares.
- `make check` + `make eval` green.
