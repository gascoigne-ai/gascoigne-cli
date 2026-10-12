status: todo

# Disable an unused MCP server from the review page

## Goal
A server nobody called in 14 days costs its whole definition set in every request. Loading tools on
request helps used servers; for a server that is unused, switching it off is the clean answer. Offer it
as a reversible choice on the review page.

## Scope
- `mcp-proxy` must know the agent's config key for its server (stats are keyed by the name the server
  reports plus a command hash; Claude names tools `mcp__<key>__tool`, and disabling needs the key). Pass it with
  the rewritten entry (`mcp-proxy -name <key> -- cmd`), accept the old form, restore both with
  `unwrap`, bump `registrationVersion` and `registrationNotes`. This touches every agent's config writer
  (`proxy.go`, `proxiedArgs`, codex tables, opencode, cursor), so do it carefully and test each.
- The action: disable the server in the agent's config for this project or for the user, recorded
  per file like our deny rules so `unwrap` and a later answer restore it exactly; never touch a server the user
  disabled. Claude first (`settings.json` and `.mcp.json`), others later.
- Browser first: a button on the review page for servers with the verdict `unused`; the writes go through
  the setup page's guard (one-time token in `setup.json`), because this changes another program's settings
  (decisions.md). `gascoigne setup` asks the same in the terminal as the fallback.

## Acceptance
- Tests: disable then restore round trip on fixture settings files, a user-disabled server untouched,
  project installs untouched by hooks, the old entry form still recognized.
