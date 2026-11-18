status: todo
kind: chore
commit: refactor: modern Go idioms
scope: cmd/**,internal/**,receiver/**,optimizer/**,inproc/**
depends_on: 025-cmd-long-functions, 026-lib-long-functions, 027-optimizer-long-functions, 031-global-state

# Modern Go sweep

Mechanical, no behavior change: `slices.Backward` (`symbols.go:221`, `blocks.go:53`, `gojson.go:80`, `fetchcache.go:76`, `prompts.go:177`, `contextnudge.go:50`), `strings.SplitSeq`/`FieldsSeq`, `maps.Copy`/`Clone` (`sdk.go:82`, `pricing.go:375`), `slices.ContainsFunc`, `errors.AsType` (`main.go:54`), tagged switches, `strings.HasPrefix` (`inproc/ledger.go:164`), `unicode.ToUpper` (`externaloptimizer/codec.go:75`), `WriteString(a + b)` -> two writes. Drop forwarding wrappers: `runProxy`, `cutGuide`, `outcomes`, `aria.render`, `testreport.with`.

Run last of tier 2 to avoid conflicts. Acceptance: `make check`, `make eval`.
