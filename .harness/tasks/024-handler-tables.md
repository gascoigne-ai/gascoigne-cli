status: todo
kind: chore
commit: refactor(optimizer): dialect handler tables over switches
scope: optimizer/**,cmd/gascoigne/hookwrap.go,internal/compress/htmlx/**,extension/ledger/**

# Handler tables over switch sprawl

Pattern exists at `taskoptimizer/task.go:21` `splitters` and `logoptimizer/log.go:84` `prePasses`. Apply to `infraoptimizer/infra.go:26` (9 cases), `k8soptimizer/k8s.go:38`, `dboptimizer/db.go:30`, `dataoptimizer/data.go:34`, `cmd/gascoigne/hookwrap.go` (29 cases beside lookup maps), `htmlx/writer.go:29` (handler per `atom.Atom`), `extension/ledger/report.go:19` (one function per grouping, picked from a table). This also removes the double dialect lookup at `k8soptimizer/k8s.go:33,38`.

Behavior identical. Acceptance: `make check`, `make eval`.
