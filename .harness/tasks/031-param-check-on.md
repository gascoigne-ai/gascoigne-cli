status: todo
kind: chore
commit: build: make check rejects one-letter parameters
scope: Makefile
depends_on: 015-names-cmd-a-c, 016-names-cmd-d-f, 017-names-cmd-g-l, 018-names-cmd-m-o, 019-names-cmd-p-r, 020-names-cmd-s-z, 021-names-extension, 022-names-httpproxy, 023-names-small-pkgs, 024-names-opt-a-f, 025-names-opt-g-k, 026-names-opt-l-z, 027-names-compress-a-p, 028-names-compress-q-z, 029-names-detect-mcp, 030-names-internal-rest

# paramnames runs in make check

All sweeps are done; add `go run ./cmd/paramnames ./...` to `make check`.

Model: unchanged.

## Acceptance
- A one-letter parameter added on purpose fails the check, then is reverted.
- `make check` passes.
