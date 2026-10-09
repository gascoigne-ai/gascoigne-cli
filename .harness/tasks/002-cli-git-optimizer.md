status: todo

# cli/git optimizer

## Goal
Shrink `git status`, `git log`, `git diff` output while keeping everything an agent acts on.

## Scope
- Package `internal/optimizer/cligitoptimizer`, type `cli`, configured as `cli/git` (id name selects
  git rules; keep room for `cli/gotest` later via config field `tool: git`).
- status: drop `(use "git ...")` hint lines and blank padding; group files by state:
  `staged: M router.go, A glob.go` style lines.
- log (default format): one line per commit `<short sha> <subject>`.
- diff: keep headers + hunks, reduce context lines to 1 (configurable `context_lines`).
- Unknown subcommand → leave payload unchanged.
- Route `git` in `configs/default.yaml` uses `cli/git`.

## Acceptance
- Eval: `git-status.txt` and `git-diff.txt` show savings, `raw ok = yes`.
- Table tests per subcommand incl. detached HEAD, merge conflicts, renames.
- `make check` + `make eval` green.
