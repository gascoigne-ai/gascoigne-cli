status: todo

# Page outline first for fetch

## Goal
The top command by tokens in the ledger is fetching documentation pages (6.1M in, 2.1M out for six
runs). Most of it is sections the agent does not need. Give the heading tree first and let the
agent fetch a section, the way `show` works for source files.

## Scope
- `fetch` (MCP tool and CLI): for a long page (above a token threshold, no query given) return the
  outline: headings with anchors and the size of each section, plus the lead paragraph. A `section`
  argument returns that section only. With a query the current ranking behavior stays.
- Library in `compress/htmlx` (heading tree, section slicing); the hint says how to ask for a section.
- Pages without headings, and short pages, behave as today. The full page stays reachable
  (`gascoigne raw <id>` or `section: all`).

## Acceptance
- Fixtures with real shapes (a long docs page, an article, a page without headings): outline
  content, anchors, sizes, section slice equals the page's section text.
- `make eval` and `html-bench.sh` do not regress; tokens for a "find one fact" task on a long page
  drop, measured with `ab.sh` (success stays 100%).
- `docs/mcp.md` and the `architecture.md` MCP section updated.
