# Agent harness

Everything an agent needs to work on gascoigne consistently. `CLAUDE.md` imports the docs;
`.claude/skills`, `.claude/agents` and `.claude/commands` symlink to `skills/`, `agents/`, `commands/`.

| Path | What |
|---|---|
| `common/` | submodule `gascoigne-harness`: shared rules, commit style, `verify` and `task-loop` skills, agent roles (see its README) |
| `rules.md` | hard rules specific to this repo (deps, architecture), on top of `common/rules.md` |
| `code-style.md` | how code should read |
| `visual-style.md` | how the dashboard and its pages look: tokens, page anatomy, blocks, pitfalls, how to screenshot (not loaded) |
| `architecture.md` | layout + index of `architecture/*.md`: flow, detection, families and dialects, mcp, llm-proxy, agents, hooks, ledger, server, reports, config, one file per area |
| `recommendations.md` | practices every change follows (loaded) |
| `backlog.md` | open ideas, measured results, dropped ideas with numbers (not loaded) |
| `skills/` | `verify` (symlink into `common/`), `add-component`, `eval`, `release` |
| `tasks/` | work queue for the task loop (empty when idle) |
| `commands/` | `task-loop.md` (symlink into `common/`) registers `/task-loop`; `.claude/commands` links here |
| `agents/` | `task-designer`, `task-implementer` (symlinks into `common/`); `.claude/agents` links here |
| `scripts/eval.sh` | token savings benchmark |
| `scripts/shot.sh` | headless Firefox screenshot of a dashboard page, to check UI changes by eye |

## Tasks
One file per task: `tasks/NNN-slug.md`, first line `status: todo`. Body = goal,
scope, acceptance criteria. Keep each task one commit's worth of work. Delete a task once
done (git keeps it); open ideas not yet worth a task live in `backlog.md`.

## Task loop
Run `/task-loop` (`--dry-run`, `--once`, or a task name) in an agent session: an orchestrator starts a
`task-designer` and a `task-implementer` subagent per task and gates scope, test and verify itself. The
verify command for this repo is `make check && make eval`. See `common/skills/task-loop/SKILL.md`.

## Eval
```sh
make eval
```
Runs every case in `testdata/eval/cases.yaml` through the `cli` pipeline against a throwaway
ledger. Prints tokens before/after per case and fails if any original is not retrievable
byte-exact.
