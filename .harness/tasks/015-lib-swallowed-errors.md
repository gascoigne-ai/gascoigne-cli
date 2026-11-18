status: todo
kind: chore
commit: fix(lib): return or log swallowed errors
scope: internal/**,optimizer/embedjsonoptimizer/**

# Stop swallowing errors in libs

Return the error, or log at debug with the reason (a deliberate fallback):
- `internal/mcpauth/handler.go:65` `store.Put`, `internal/mcpauth/mcpauth.go:58` `json.Unmarshal` (corrupt login file reads as empty)
- `internal/exact/o200k.go:66,70` `os.Remove`, `os.WriteFile`
- `internal/pricing/pricing.go:174` `json.MarshalIndent`
- `internal/service/service.go:123` `component.ParseID`
- `optimizer/embedjsonoptimizer/embedjson.go:75` `jsonx.Render`

Test first for each that changes behavior (corrupt file, failing store). Acceptance: `make check`; `make eval` for the embedjson change.
