status: todo

# Permission and update decisions through a dashboard page

## Goal
Ask the user in a web page, not a terminal prompt, about: fewer approval prompts for gascoigne's
tools (task 033), keeping installed hooks updated (task 035). Nothing is granted silently.
Needs tasks 033 and 035.

## Scope
- Pending decisions come from `prefs.json`: `allow` and `autosync` unset = undecided. `install`
  and the auto-sync (035) open `http://<server>/setup?token=…` with `openBrowser` when something is
  pending, at most once per `registrationVersion`, never when `CI` is set, no display, or
  `GASCOIGNE_SERVER=off`; those fall back to flags and a one-line notice in the guide.
- Routes in `dashboard.go`: `GET /setup` (embedded `setup.html`), `GET /api/setup` (pending items,
  plain-language text: what, which file, which agent, effect, default), `POST /api/setup`
  `{token, allow, autosync}`.
- Token: random, written to `setup.json` in the state dir by `install`, checked and deleted on a
  successful POST, expires after 24h. Only the ids offered are accepted, never rules or paths from
  the request. The existing loopback-Host and cross-origin checks stay.
- Apply: store the answers, re-register user-scope installs (task 035), report project-scope installs
  that need `gascoigne install` run in the project.
- `gascoigne setup`: the same questions in the terminal (`-y` takes the defaults).

## Acceptance
- Handler tests: no token, wrong token, expired, replay, valid; page lists only pending items.
- Architecture §Agents and §Ledger/Dashboard updated.
