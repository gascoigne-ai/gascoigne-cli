status: blocked
kind: feature
commit: refactor(component): split the Ledger port
scope: component/**,extension/ledger/**,internal/service/**,internal/mcptool/**,.harness/architecture/**
depends_on: 033-unexport-unused-api

# Split the 6-method Ledger port

`component/ledger.go:133` `Ledger` has 6 methods (guideline: about five). Three more capabilities are found by type assertion (`UsageRecorder`, `OutputHistory`, `ToolStatRecorder`). 14 interfaces overall vs the 6-port guideline.

First run the `data-model` audit skill and report; change nothing until the user agrees. Then split into `RawStore` (`SaveRaw`, `GetRaw`), `Recorder` (`Record`, `MarkFailure`, `Failures`), `StatsSource`, type `Stat.Outcome` as `Outcome` (`component/ledger.go:76`), and add `Grouping` and `FailureReason` types for `StatsQuery.By` and `MarkFailure`.

CORE CONTRACT CHANGE: needs explicit user approval. Persisted ledger format and its version stay unchanged, or get a migration plus fixture.

## Model
Write the design note: access pattern per caller (receivers write, reports read, retrieve reads raw), the structures chosen, the rejected single wide port, and the signal to revisit.

Acceptance: `make check`, `make eval`, `SUITES=ledger make eval`.
