# Full-payload guarantee

Lives in core (`service/pipeline.go`), not in optimizers:

1. Before optimizers run, core snapshots the payload.
2. Output saving fewer than `minSaving` (20) tokens, the cost of the retrieve hint, → the original
   is kept, outcome unchanged. If the output differs, the original goes to the ledger first,
   `raw_id` is set, and stdout gets the hint
   `[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]`.
3. Ledger save fails → the original is returned. Optimizer errors → the original is returned and
   a failure recorded.
4. Up-front opt-out: the `bypass` filter (`-raw`, `GASCOIGNE_RAW=1`, attr `raw=true`).
5. Retrieval: `gascoigne raw <id>`, MCP `retrieve` / `gascoigne_retrieve` (later: HTTP proxy
   `GET /raw/<id>`).
