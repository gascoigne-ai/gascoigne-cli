status: todo

The `history` filter bypasses a source forever after one failure. On this machine one fetch URL that
needed a full `raw` read once passed through unoptimized on two later fetches and cost 2M tokens, 13%
of a month's savings.

Scope: `internal/filter/historyfilter/history.go:44-49` ignores `LastTS` (`extension/ledger/models.go:52-57`).
Count a failure only inside a `failure_ttl` (about 24h) and/or raise `max_failures` to 2
(`configs/default.yaml:32`). A full retrieve of a fetch URL weighs less than one of a command
(`internal/mcptool/mcptool.go:86-96`): a doc page is read whole by design. Add a column to
`gain -failures`: tokens passed raw since the key failed.

Done when: unit test for the TTL and weight; `make check` and `make eval` show unchanged savings and
`raw ok` never `NO`; `.harness/architecture/` filters doc updated in the same commit.
