status: todo
kind: docs
commit: docs(harness): CLAUDE.md imports data-structures.md
scope: CLAUDE.md

# CLAUDE.md imports data-structures.md

The shared `data-structures.md` is part of the harness but CLAUDE.md does not import it, so sessions never load it. Add `@.harness/common/data-structures.md` to the Harness import list.

Model: unchanged.

## Acceptance
- `grep data-structures CLAUDE.md` finds the import.
- `make check` passes.
