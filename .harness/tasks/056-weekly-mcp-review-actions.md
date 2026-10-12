status: todo

# Weekly MCP review: the nudge

## Goal
The review page and the lazy-tools choice exist (`gascoigne mcp-review`). Make looking at it a weekly
habit with little effort: one session-start page and note when there is something worth saying.

## Scope
- A claude SessionStart (startup or resume, like `releaseNudge` and the setup page) opens the page
  (`/mcp`, `openBrowser`) at most once every 7 days (`prefs.last_mcp_review`, read before anything
  else), and adds a one-line note with the numbers.
- One browser page per session start: the setup page goes first, then the release page, then this one.
  Never two at once.
- Trigger only on what the user can act on: proxied servers (with sizes) that are `unused`, or whose
  idle tools, not yet chosen to load on request, add up to more than a threshold of tokens per request
  (named constant, start at 1,500). Direct servers never trigger it: they cannot be called unused.
- Ask the background server (`GET /api/mcp`, loopback, short timeout) instead of opening the ledger in
  the hook; any failure means no nudge and never fails the hook. Not under `CI`, without a display, or with
  `GASCOIGNE_SERVER=off`: the note alone, pointing to `gascoigne mcp-review`.
- Closing the tab ends it until next week. Off: `prefs.mcp_review: off` and `GASCOIGNE_MCP_REVIEW=off`;
  an optional setup question like `release_check`.
- `architecture.md` Agents section and `docs/agents.md` updated in the same commit.

## Acceptance
- Tests: cadence (a week), the server failing or slow, nothing to say, a page opened by another
  nudge in the same start, CI and no display, opt-out, a project install untouched.
