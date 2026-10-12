status: todo

# Fold link-dense blocks in web pages like nav

## Goal
Article benchmark (`html-bench.sh`, 181 pages): recall 98.3%, precision 59%. Mega-menus written as
plain link lists (not `<nav>`) fill the doc budget before the story, and share bars and footers
outside `<footer>` cost precision.

## Scope
- `compress/htmlx`: a block whose text is mostly link text (high link-text share, many short items)
  folds the way `<nav>` does: one line with the count and the first few labels.
- Thresholds as named constants, tuned on the bench; no new config keys.
- Pages the misses came from (news site mega-menus, government portals) go in as fixtures with a
  real shape and no real user data.

## Acceptance
- `html-bench.sh`: recall must not drop below 98.3%; precision up. Report both in the commit.
- `make eval`: savings must not regress and `raw ok` never `NO`.
- Unit tests: a link list folds, a prose paragraph with links does not, a table of contents that
  the query asks about keeps its matching items.
