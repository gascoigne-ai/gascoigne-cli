status: todo
kind: docs
commit: docs(code): godoc on exported core identifiers
scope: component/**,payload/**,internal/**,extension/**,filter/**,receiver/**,exporter/**,optimizer/**

# Doc comments on exported identifiers

Missing on: `component/component.go` (`Kind`, `ParseID`, `Host`, `Component`, `Receiver`, `Decision`, `Filter`, `Exporter`, `Settings`), `component/factory.go`, `component/ledger.go`, `payload/payload.go`, `internal/config/config.go`, `internal/service/{service,pipeline}.go`, `internal/router/router.go`, `extension/ledger/ledger.go`, `internal/detect/detect.go`; and each component `Config`, `Receiver`/`Filter`/`Exporter`/`Optimizer` type and `NewFactory` (23 optimizer packages lack `Config` docs). Say what it is or does, not what the code says. Named const for `5*time.Second` at `httpproxyreceiver/usage.go:152`.

Comments only, so `make check` runs gofmt and vet.
