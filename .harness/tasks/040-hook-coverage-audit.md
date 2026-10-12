status: todo

# Hook coverage audit

## Goal
Know where optimization does not reach, and close cheap gaps.

## Scope
- Check that installed hooks fire inside Claude subagents and report what does not.
- Cursor: a post-tool output replacement if its hook API allows; else document.
- Stop hook or `gain -oneline` for a session summary of savings.
- List built-in tools that bypass shell hooks (Read, Grep, Glob, web fetch) and what covers each today.

## Acceptance
- A short table in architecture §Agents (agent × tool × covered by); each gap closed or recorded in
  `recommendations.md` Open.
