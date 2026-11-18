status: todo
kind: chore
commit: refactor(cmd): group dashboard routes and split setup.go
scope: cmd/gascoigne/setup*.go, cmd/gascoigne/dashboard.go, .harness/architecture/reports.md, .harness/architecture/server.md, .harness/architecture/agents.md

# Split setup.go and dashboard()

- `dashboard.go` `dashboard()` (:55) is 203 lines with about 35 inline route closures. `setupHandlers(mux)`
  and `pricingHandlers(mux)` show the pattern: group into `reportHandlers`, `mcpHandlers`,
  `ledgerHandlers`, `assetHandlers`; static pages and assets (:89-:121) become one table of path, content
  type and bytes.
- `setup.go` (849 lines): `setup_decisions.go` (`decisions`, `experiments` table, `applyAnswers`),
  `setup_token.go` (setup token and link), `setup_http.go` (handlers and terminal prompt). Make
  `applyAnswers` (81 lines) a table of prefix to handler.

Steps: add tests for route registration (every current path answers) before moving; then move code.

Acceptance: `make check` passes; the route list is identical before and after; docs updated in the same
commit. UI-facing: run `scripts/shot.sh` on one page.
