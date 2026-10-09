status: todo

# exec receiver + `gascoigne exec` command

## Goal
`gascoigne exec -- git status` runs the command and prints optimized output, so agent hooks can
rewrite shell commands to go through gascoigne.

## Scope
- Package `internal/receiver/execreceiver` (see skill add-component). Config: `kind` (default `cmd_output`).
- Receiver runs argv from attrs/flags, captures stdout as one block, sets `cmd` attr (argv joined).
- stderr streams through untouched; exit code of the child is preserved by `gascoigne exec`.
- New CLI subcommand `exec` in `cmd/gascoigne` using pipeline `exec` from config.
- Add `exec` pipeline to `configs/default.yaml` (same filters/router as `cli`).

## Acceptance
- `bin/gascoigne exec -- sh -c 'echo hi; exit 3'` prints `hi`, exits 3.
- `bin/gascoigne exec -raw -- ls` output byte-identical to `ls`.
- Table tests for exit codes, stderr passthrough, missing binary error.
- `make check` green.
