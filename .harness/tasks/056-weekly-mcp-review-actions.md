status: todo

# Weekly MCP review: weekly nudge and safe actions

## Goal
Make the review a habit with little effort: a once-a-week note, and two reversible actions the user
can accept: disable an unused server, or hide tools of a used server that were never called.

## Scope
- Weekly nudge: a claude SessionStart note (startup or resume, like `releaseNudge`), at most once
  every 7 days (`prefs.last_mcp_review`), only when task 055 finds something worth saying
  (an unused server, or hidden-tool savings above a threshold in tokens per request). One line with
  the numbers and `gascoigne mcp-review`. Off: `prefs` and an env var. Never opens a page by itself.
- Setup question (`decision` in `setup.go`, group "tools", default no change) per finding:
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
- Tests: nudge cadence and opt-out, disable then restore round trip on fixture settings files
  (claude `settings.json` and `.mcp.json`, codex `config.toml`), a user-disabled server untouched,
  hidden tool still callable by name through the proxy, project installs untouched by hooks.
- `architecture.md` Agents section and `docs/agents.md` updated in the same commit.
- Depends on tasks 041 and 055.
