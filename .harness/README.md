# Agent harness

Everything an agent needs to work on gascoigne consistently. `CLAUDE.md` imports the docs;
`.claude/skills` symlinks to `skills/`.

| Path | What |
|---|---|
| `common/` | submodule `gascoigne-harness`: shared rules, commit style, `verify` skill, `loop.sh` (see its README) |
| `rules.md` | hard rules specific to this repo (deps, architecture), on top of `common/rules.md` |
| `code-style.md` | how code should read |
| `architecture.md` | layout + index of `architecture/*.md`: flow, detection, families and dialects, mcp, llm-proxy, agents, hooks, ledger, server, reports, config, one file per area |
| `recommendations.md` | practices every change follows (loaded) |
| `backlog.md` | open ideas, measured results, dropped ideas with numbers (not loaded) |
| `skills/` | `verify` (symlink into `common/`), `add-component`, `eval`, `release` |
| `tasks/` | work queue for the autonomous loop (empty when idle) |
| `scripts/loop.sh` | wraps `common/scripts/loop.sh` with `make check && make eval` |
| `scripts/eval.sh` | token savings benchmark |
| `logs/` | loop transcripts (gitignored) |

## Tasks
One file per task: `tasks/NNN-slug.md`, first line `status: todo`. Body = goal,
scope, acceptance criteria. Keep each task one commit's worth of work. Delete a task once
done (git keeps it); open ideas not yet worth a task live in `backlog.md`.

## Autonomous loop
```sh
.harness/scripts/loop.sh --dry-run   # list pending tasks
.harness/scripts/loop.sh --once      # do next task only
.harness/scripts/loop.sh             # work through all todo tasks
```
Per task: headless `claude -p` with the task + harness rules → `make check` + `make eval`
→ on success deletes the task file and commits; on failure stops and prints the log path.
Env: `MAX_TURNS` (default 50), `CLAUDE` (binary, default `claude`).

## Eval
```sh
make eval
```
Runs every case in `testdata/eval/cases.yaml` through the `cli` pipeline against a throwaway
ledger. Prints tokens before/after per case and fails if any original is not retrievable
byte-exact.
