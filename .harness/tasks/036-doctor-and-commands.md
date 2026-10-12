status: todo

# `gascoigne doctor` and agent-side commands

## Goal
No way to tell why gascoigne is not working in a session (hooks missing, MCP not approved, server
down, stale registration), and no in-agent shortcut to see savings.

## Scope
- `gascoigne doctor [--fix]`: per agent, whether the config registers hooks, MCP, deny/allow rules,
  guide; server state; proxy env; registration number vs `registrationVersion`; unapproved or
  untrusted state where detectable. `--fix` re-runs `install` for what is stale.
- Claude: `install` also writes a skill and slash commands (`gascoigne-stats` runs `gain`,
  `gascoigne-doctor`), recorded and removed by `unwrap`. Other agents: a line in the guide block.
- Optional: `gain -oneline` for a status-line command showing session savings.

## Acceptance
- Table tests with fake config trees (healthy, missing hook, stale registration, bad JSON).
- Help text, README/docs and architecture §Agents updated.
