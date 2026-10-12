status: todo

# Weekly MCP review: report

## Goal
Show, once a week, what the user's MCP servers cost per request and which of them were never used,
with numbers from task 041. Report only; acting on it is task 056.

## Scope
- **Browser first.** The review is a page in the dashboard (`/mcp`, embedded HTML like
  `dashboard.html`, no build step, same loopback-Host and cross-origin checks): a sortable table
  (accessor-based sort, decisions.md), a bar for each server's share of per-request tool tokens,
  the 7 days against the 7 before as a small trend, and per-tool detail folded under each server.
  `gascoigne mcp-review` opens it (`openBrowser`; `-open=false` only prints the URL). Without a
  display, under `CI` or with `GASCOIGNE_SERVER=off` it prints the same table in the terminal
  (`-json` for scripts), plus `gain` section "mcp servers" and MCP `stats` (plain text).
- One row per server: tools, schema tokens per request, calls in the last 7 days, calls in
  the 7 days before, last used. Verdict per server: `unused` (no calls in 14 days), `rarely used`
  (calls, but cost per call far above the median), `used`. A tool-level list for used servers:
  tools never called in 14 days and their tokens.
- Cost line: schema tokens times requests in the window, priced with `cost` at the model's cache-read
  price ("server X: 9k tokens per request, about $Y a week, 0 calls"). Labeled estimate (decisions.md).
- Honest gaps: servers that stay direct have calls but no sizes unless the LLM proxy saw them;
  say so in the row, do not guess a size. Fewer than 14 days of data: say how many days exist.
- `doctor` prints one line when a server is `unused`, pointing to `mcp-review`.

## Acceptance
- Fixture ledger tests: unused, rarely used, used, only-hook-counted, young ledger (3 days of data),
  empty ledger.
- Output follows `render.go` conventions (`-color`, no ANSI in MCP `stats`).
- `docs/cli.md` and `help.go` entry in the same commit.
