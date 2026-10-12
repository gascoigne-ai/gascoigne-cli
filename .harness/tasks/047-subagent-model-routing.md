status: todo

# Offer a cheaper model for read-only exploration agents

## Goal
Wide exploration (many reads and searches) runs on the main model and its price. A read-only
subagent on a cheaper model does the same job for a fraction of the cost, and its results stay out
of the main context. `gain -by model` already shows the split.

## Scope
- Setup question (`decision` in `setup.go`, optional group, default no): write a user-scope Claude
  agent definition for read-only exploration (read, search and read-only shell tools) with a cheaper
  model. Marked like our skills, so `unwrap` removes only ours.
- Guide line, only when the agent exists: delegate wide searches to it.
- Bump `registrationVersion` and `registrationNotes`. Nothing changes without an answer
  (decisions.md).

## Acceptance
- Tests: the file is written and removed exactly, an existing agent of the same name is never
  overwritten, the guide text appears only with the agent.
- `ab.sh` on tasks that read dozens of files: success stays 100%, cost per task and main-context
  tokens drop. Record in `backlog.md`.
