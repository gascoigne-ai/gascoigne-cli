status: todo

# Keep installed hooks current after an upgrade

## Goal
Installed hooks call `gascoigne hook <agent>` by binary path, so hook logic updates with the binary;
the registration (events, matchers, guide text, rules) goes stale, so a new hook never reaches
existing installs.

## Scope
- `install` records `installs.json` in the state dir: agent, scope, dir (project), exe, and a
  `registration` number; `registrationVersion` is a const bumped whenever a hook/matcher/guide/rule
  set changes. `unwrap` drops its entries.
- Sync on claude SessionStart and codex/opencode prompt hooks (cheap JSON read): for user-scope
  installs whose number is older, re-run the agent's registration. Claude re-registers settings only
  (`addClaudeSettings`), never `claude mcp` or `~/.claude.json`, which a running Claude Code rewrites.
  Project-scope files are never edited unprompted; `doctor` warns.
- Only when `prefs.autosync` is `on` and `GASCOIGNE_AUTOSYNC` is not `off`. Replaces only
  gascoigne's own entries, keeps `.gascoigne-bak`, takes a lock, never fails the hook, tells the
  agent once ("hooks updated; restart for new ones"). `install --sync` does it by hand.
- Flag `install -autosync on|off` stores the answer; the question itself belongs to task 034.

## Acceptance
- Tests: stale and current manifest, off, project scope skipped, concurrent syncs, hook never errors.
- Architecture §Agents "Hooks" updated.
