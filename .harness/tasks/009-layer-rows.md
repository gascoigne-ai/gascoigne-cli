status: todo
kind: chore
commit: build(harness): layer table zones cmd, inproc and configs
scope: `.harness/layers.txt`, `.harness/architecture.md`

# Every package belongs to a layer zone

`check-layers.sh` skips packages in no zone, and `cmd/gascoigne`, `cmd/mcpmock`, `inproc` and `configs` are in none. Add an app row (`./cmd/...`, `./inproc`) that may import every other zone and a leaf assets row for `./configs`; add the assets zone to the may-import list of zones that use it.

Model: unchanged.

## Acceptance
- Every package in `go list ./...` falls in a zone.
- A deliberate import from `configs` into `internal/config` fails the layer check, then is reverted.
- `make check` passes.
