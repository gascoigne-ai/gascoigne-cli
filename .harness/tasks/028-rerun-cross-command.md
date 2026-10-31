status: todo

`rerun` folds only runs of the same command key (`rerunoptimizer/rerun.go:53`, `p.Key()`). A
`go test ./...` followed by `go test -run X ./pkg` showing the same failure prints it twice.
Local data says identical-output Bash reruns are only 0.5%, so this is low priority.

Scope: first measure how often a different command repeats lines of an earlier result in the same
session. If worthwhile, add a session-wide reference for test/build families via the ledger
(`RecentOutputs` by session, capped), modelled on `receiver/httpproxyreceiver/fold.go`. Pointer names
the first line of the earlier output.

Done when: measurement recorded in backlog; if built, `make check` and `make eval` pass and the
architecture doc is updated in the same commit.
