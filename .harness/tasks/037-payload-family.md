status: blocked
kind: feature
commit: refactor(payload): one source for family from type
scope: payload/**,internal/detect/**,filter/historyfilter/**,internal/mcptool/**

# payload.Family as the single source

Family-from-type is re-implemented in `detect.Family` (`detect.go:60`), `historyfilter/history.go:86`, `mcptool.go:375`, `payload/payload.go:116`. Add public `payload.Family`; `detect.Family` forwards, the others call it.

CORE CONTRACT CHANGE (`payload/`): needs explicit user approval.

## Model
One derived fact, computed not stored. Rejected: a stored family field beside the type.

Acceptance: `make check`.
