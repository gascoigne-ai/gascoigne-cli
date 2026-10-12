status: todo

# Read size guard without the proxy

## Goal
A claude.ai login stays off the LLM proxy, so a Read of a lock file, generated code or a 3k-line
source enters the context whole and is re-read every turn. A PreToolUse hook can stop it for
every user.

## Scope
- Claude PreToolUse on `Read`: no `offset`/`limit` and the file is above a token threshold (config,
  default about 4000) -> deny with a reason holding the file's outline (`show` definitions with
  line ranges) and "read the range you need". Never for files the agent just edited, never when
  `limit` is set, never for images or PDFs.
- Registration: new matcher, bump `registrationVersion` and `registrationNotes`; `unwrap` removes
  it. Must not break Edit, which needs a prior Read: the reason has to say a ranged Read counts.
- Second step, only if Claude Code accepts `updatedToolOutput` for Read: replace the result using
  the proxy's Read path (split `N\t`, optimize as `cat`, `renumber`).

## Acceptance
- Hook shape tests (deny, allow, small file, ranged Read, missing file).
- `ab.sh` on edit tasks over big files: success stays 100%, tokens per task drop.
- `architecture.md` Agents section and coverage table updated in the same commit.
