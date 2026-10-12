status: todo

# Settings and plugin cost audit in doctor

## Goal
Per-session and per-prompt costs outside tool output: plugin hook context, autocompact threshold,
thinking level, auto-memory, big resumes. Users cannot see them. Price each like `prompts` prices
instruction files.

## Scope
- `doctor`: for each hook in the agent's settings that injects context (SessionStart,
  UserPromptSubmit), run it or read its recorded output and report tokens per session and per
  prompt. Report only, one suggested fix per line.
- Flag settings known to cost tokens (list kept short, each with its effect and source).
- `prompts` shows the same hook-context rows next to CLAUDE.md and memory.
- Nothing is changed (decisions.md: found things are signaled, never acted on).

## Acceptance
- Tests with fixture settings files; hooks that fail or time out are reported, not fatal.
- No home paths or real user data in fixtures.
