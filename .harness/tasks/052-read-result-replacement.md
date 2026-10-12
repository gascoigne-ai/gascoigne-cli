status: todo

# Replace Read results after the call

## Goal
Reads are never replaced by the hooks because Claude numbers their lines. The proxy already
handles this (split `N\t`, optimize like `cat`, restore the numbers). Users without the proxy keep
paying for whole reads. Bring the same result to the PostToolUse hook, if the agent allows it.

## Scope
- First step, no code: check on a real session whether Claude Code accepts `updatedToolOutput` for
  `Read`, and in what shape. Record the answer in `architecture.md` (coverage table). If it does
  not, close this task with that note and stop.
- If it does: PostToolUse matcher for `Read`, reuse the proxy's `renumber` logic through a shared
  library (move it to `compress/` rather than importing a component), `auto` mode files only, never
  for a file the agent edited in the session, skip hinted output.
- Bump `registrationVersion` and `registrationNotes` (new matcher). `unwrap` removes it.

## Acceptance
- Hook shape tests; the kept lines carry their original numbers; Edit still works on a shortened Read.
- `ab.sh` on edit tasks over large files: success 100%, tokens per task down.
- `architecture.md` Agents section and coverage table updated in the same commit.
