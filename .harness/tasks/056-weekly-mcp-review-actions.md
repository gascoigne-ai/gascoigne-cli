status: todo

# Weekly MCP review: weekly nudge and safe actions

## Goal
Make the review a habit with little effort: a once-a-week note, and two reversible actions the user
can accept: disable an unused server, or hide tools of a used server that were never called.

## Scope
- **Browser first.** The review and its choices live on the page of task 055, not in the terminal.
  A claude SessionStart (startup or resume, like `releaseNudge` and the setup page) opens it with
  `openBrowser` at most once every 7 days (`prefs.last_mcp_review`), only when task 055 finds
  something worth saying (an unused server, or hidden-tool savings above a threshold in tokens per
  request), and adds a one-line note with the numbers. Closing the tab without answering ends it
  until next week (no nagging, unlike the setup page). Not under `CI`, without a display, or with
  `GASCOIGNE_SERVER=off`: the note alone, with `gascoigne mcp-review`. Off: `prefs` and an env var.
- Choices are saved through the setup page's guard (one-time token in `setup.json`, constant-time
  compare, only offered ids and values accepted); `gascoigne setup` asks the same in the terminal
  as the fallback. Each finding is a question (`decision` in `setup.go`, group "tools", default no
  change) with a sentence saying what each option does and what it saves:
  - **Disable server** in the agent's config for this project or for the user, recorded per file
    like our deny rules (`unwrap` and a later answer restore it exactly; never touch a server the
    user disabled).
  - **Hide never-called tools** of a server that runs through `mcp-proxy`: extend the proxy config
    (`max_tools` exists; add a deny list of tool names). Hidden tools are listed by one extra proxy
    tool (name and one line each) and can be called after being named, so nothing becomes
    unreachable; the answer is reversible with `unwrap` or a new answer.
- Nothing changes without an answer (decisions.md). Bump `registrationVersion` and
  `registrationNotes` if what `install` writes changes.

## Acceptance
- Tests: page opens once a week and not under CI or without a display, bad or reused token refused,
  nudge cadence and opt-out, disable then restore round trip on fixture settings files
  (claude `settings.json` and `.mcp.json`, codex `config.toml`), a user-disabled server untouched,
  hidden tool still callable by name through the proxy, project installs untouched by hooks.
- `architecture.md` Agents section and `docs/agents.md` updated in the same commit.
- Depends on tasks 041 and 055.
