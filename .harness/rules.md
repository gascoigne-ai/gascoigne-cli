# Rules

Adds to the shared `common/rules.md` and `common/commit-style.md`. Breaking one = change rejected.

## Dependencies
- Stdlib first. Approved third-party deps:
  - `go.yaml.in/yaml/v3` — config
  - `gorm.io/gorm` — ledger ORM
  - `github.com/ncruces/go-sqlite3/gormlite` — gorm sqlite dialect, pure Go (wasm), no cgo.
    Never swap in a cgo driver (`gorm.io/driver/sqlite`, mattn): builds must stay `CGO_ENABLED=0`.
  - `golang.org/x/net` — HTML parsing (`html`, `html/atom`) for web data
  - `github.com/modelcontextprotocol/go-sdk` — MCP server/client
  - `github.com/ebitengine/purego` — opens the ONNX Runtime library at run time without cgo (`internal/prose/ortrt`)
  - `golang.org/x/text` — NFC normalization for the prose model tokenizer (`internal/prose/bpe`)

## Verification
- `make check` (gofmt, vet, imports, tests) must pass before you call work done.
- Every bug found (by you, the user or a failing run) starts with a regression test: write it first, run it and see it fail for the reported reason, only then fix. Report that it failed before and passes after. A bug that cannot be reproduced in a test is not understood yet: keep investigating or say so; do not ship a guess as a fix. Put the test beside the code it covers, named for the behavior (`TestFillDaysEndsWithToday`), with a comment saying what went wrong; a data bug also gets an eval or fixture case when a suite fits.
- Optimizer changes: run `make eval`; savings must not regress and `raw ok` must never be `NO`.
- MCP changes (`receiver/mcpproxyreceiver`, `internal/mcptool`, MCP review, anything that shrinks an MCP tool result or tool list): also run the `mcp` eval suite (`make eval-mcp`). A new result shape or tool gets a generator in `internal/mcpmock` and a case in `testdata/eval-mcp/cases.yaml`; check against a real MCP server by hand (`gascoigne mcp-proxy -- <server>`) before release when one is at hand.

## Architecture
- Extend by adding a component (receiver, filter, optimizer, exporter, extension) + config.
  Never special-case a component inside core.
- Core = public contracts `component/`, `payload/`, `tokens/` + engine `internal/{config,router,service}`.
  Changing core needs user approval. Contract changes break every component — treat as API changes.
- Boundary: public contracts `component/`, `payload/`, `tokens/` never import `internal/`. Component
  dirs (`receiver/`, `filter/`, `optimizer/`, `exporter/`, `extension/`) may import them and the
  libs in `internal/` (`compress`, `detect`, ...), never the engine `internal/{config,router,service,registry}`
  (enforced by `make imports`). No imports between component packages.
- New optimizer = new package + registry line + route in `configs/default.yaml` + eval case.
- Never lose information. Optimized output must stay reversible through the ledger;
  bypass output stays byte-exact. Optimizers never write to the ledger themselves — core does.
- Optimizers are deterministic: same input, same output (prompt caching + eval depend on it).

## Hygiene
- Competitor names (`common/rules.md`) are allowed also in `testdata/*/NOTICE.md` and for the Hugging
  Face ids of the prose models (`internal/prose/models.go`, `docs/prose-model.md`), which are what
  users select and download.
- No secrets, real user data or home paths in fixtures.

## Worktrees
- The background server defaults to `127.0.0.1:7878`; set `GASCOIGNE_SERVER_ADDR` to a free port when
  agents run concurrently. `bin/` is ignored: `make build` per worktree.
