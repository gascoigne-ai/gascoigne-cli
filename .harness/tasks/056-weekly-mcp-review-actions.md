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
  - **Load never-called tools on request** for a server that runs through `mcp-proxy`. Nothing is
    hidden for good: the agent is told the tools exist and asks for them explicitly.
    - `tools/list` keeps the tools that were used and replaces the rest with one proxy tool per
      server, `gascoigne_more` (extend `compact_tools` / `max_tools` config with a per-server
      `lazy` list). Its description names the lazy tools, names only and no schemas, e.g.
      "12 more tools of <server>: a, b, c. Ask for one by name to get its schema and call it".
    - Asking is a call to that tool with a tool name: it returns the schema and description, and a
      second form (`call`) runs the tool with arguments, passing the request to the server
      unchanged. The tool set the client sees never changes during a session, so the prompt cache
      stays intact (no `list_changed`).
    - The guide gets one line saying lazy tools exist and are fetched only when the task needs one.
      Nothing is loaded without the agent asking.
    - Reversible: the answer on the page, `unwrap`, or the config list.
- Nothing changes without an answer (decisions.md). Bump `registrationVersion` and
  `registrationNotes` if what `install` writes changes.

## Acceptance
- Tests: page opens once a week and not under CI or without a display, bad or reused token refused,
  nudge cadence and opt-out, disable then restore round trip on fixture settings files
  (claude `settings.json` and `.mcp.json`, codex `config.toml`), a user-disabled server untouched,
  a lazy tool returns its schema when asked and runs through `call` with the server's own result and
  errors unchanged, the listed tool set is byte-identical across calls, names-only description stays
  under a token budget per server, project installs untouched by hooks.
- Cost check, offline: tokens of the lazy listing against the full schemas on the fixture server.
- `architecture.md` Agents section and `docs/agents.md` updated in the same commit.
- Depends on tasks 041 and 055.
