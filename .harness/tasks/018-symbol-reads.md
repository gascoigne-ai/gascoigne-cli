status: todo

# Symbol reads

## Goal
Agents read whole files to find one function.

## Scope
- `gascoigne show file:Symbol` and MCP `show` tool: one definition with its original line
  numbers (header `file:L40-88`), built on catoptimizer's outline logic (braces/indent).
- `gascoigne outline file`: signatures with line numbers.
- Agent guide line for both.

## Acceptance
- Table tests for Go, Python, TS, Rust; unknown language → clear error.
- `make check` green.
