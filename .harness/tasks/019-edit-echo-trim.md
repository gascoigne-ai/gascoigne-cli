status: todo

# Proxy: trim edit-tool result echoes

## Goal
Edit tools return a numbered snippet of the file around the change, mostly lines already in context.

## Scope
- Per agent result shape (claude Edit/MultiEdit/Write), keep changed lines ±2 with original
  numbers; rest → one marker. Measure with `gascoigne replay` first; drop if < 1% of tool
  tokens.

## Acceptance
- replay report before/after; tests on real-shaped results; `make check` green.
