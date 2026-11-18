status: todo
kind: chore
commit: refactor: unexport symbols with no outside user
scope: internal/**,filter/**,receiver/**,inproc/**

# Unexport symbols used only inside their package

Verified in-package only (re-grep, also `gascoigne-sdk` and `gascoigne-jarnam`): `service.PipelineNames`, `mcptool.StatsTable`, `detect.FromFile`, `detect.FromMIME`, `mcpauth.LoginTimeout`, `router.DefaultRoute`, `cost.PriceSheetVersion`, `pricing.DefaultTimeout`, `bypassfilter.DefaultEnv`, `detectfilter.AttrMixed`/`MixedTrue`, `httpproxyreceiver.RawHeader`. Move test-only `mcpproxyreceiver.WithTransport` to `export_test.go`. Keep `externaloptimizer` wire types (protocol docs) and the `inproc` API.

Acceptance: `make check`.
