status: later

# Dashboard tables: sorting and filtering

## Idea
The dashboard tables (by command, optimizer, content type, model family, model, skipped/failed/
unchanged reasons, excluded sources, recent runs) show rows in the server's order with no way to
reorder or narrow them. Long lists (many commands, many models) are hard to scan.

## Sketch
- Click a column header to sort (ascending, descending, off); numbers sort by value, not by the
  `12.3k` text; the sort stays per table while the page lives (and in the URL hash, so a link
  keeps it).
- A small text box above each long table filters rows by substring across its text columns.
- Build it once in the shared `table`/`lazyTable` helpers of `dashboard.html`, so every table gets
  it; keep row virtualization working (sort/filter the data, not the DOM rows).
- Live updates (server-sent events for the feed) must not reset the user's sort or filter.
- Server side: nothing for the summary tables (data is already in the page); the recent-runs feed
  filters by the existing `outcome` / session / agent / model params, plus a command text filter
  if the page cannot hold enough runs.

## Open questions
- Which tables are long enough to need it (probably commands, models, reasons, feed)?
- Keyboard and mobile behavior of the header controls.

## Acceptance (when picked up)
- Tests on the number parsing / comparator helpers if moved to Go-testable code, else a manual
  checklist in the PR; no new dependency, no build step.
- Architecture §Dashboard updated.
