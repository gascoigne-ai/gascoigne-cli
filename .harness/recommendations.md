# Recommendations

Practices for every change. Rules are in `rules.md`, the reasons behind design choices in
`decisions.md`; this is advice. Open ideas, measured results and dropped ideas are in
`backlog.md` (not loaded). Work for the autonomous loop goes in `tasks/` (one file per task,
`status: todo`); finished tasks are deleted, their record is git history.

## Practices
- New tools of an existing family are dialects, not optimizers (skill `add-component`); shared
  primitives go to `compress/`.
- Deterministic, idempotent output ⇒ prompt-cache friendly: no timestamps or random ids.
- Measure before claiming: every optimizer gets eval cases with real-shaped fixtures.
- Keep hint/marker text short; it costs tokens too.
- Never transform for whitespace alone (runs of spaces/newlines are one token each);
  comments are where source tokens are (14–50%).
- Prefer stdlib; package-level regexps via `lazyre.New`, sparingly.
