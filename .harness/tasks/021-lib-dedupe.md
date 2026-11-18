status: todo
kind: chore
commit: refactor(lib): share cache, http get and small helpers
scope: internal/**,receiver/**,optimizer/traceoptimizer/**

# Dedupe helpers in libs and receivers

- `symbols` qualify loop: `symbols.go:220`, `blocks.go:53` -> `qualify(...)`.
- One ANSI regex: `compress/compress.go:12`, `detect/sniff.go:16`.
- Generic bounded `internal/lru` for `httpproxyreceiver/results.go` (LRU) and `mcpreceiver/fetchcache.go` (FIFO with `order[1:]` pinning the array).
- One bounded HTTP GET helper for `exact/o200k.go`, `prose/install.go`, `pricing/pricing.go:494`.
- Shared receiver `Validate` (`execreceiver`, `hookreceiver`, `stdinreceiver` config).
- Drop `re()` wrappers: `diag/dialects.go`, `pkgnoise/dialects.go`, `traceoptimizer/frames.go:23`.
- One message walker in `httpproxyreceiver` (`formats.go`, `skills.go`, `prompts.go`).

If this grows past one commit, split by bullet. Acceptance: `make check`, `make eval`.
