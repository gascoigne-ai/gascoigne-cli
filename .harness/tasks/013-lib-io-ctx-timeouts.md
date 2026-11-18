status: todo
kind: chore
commit: fix(prose): bound I/O with ctx and timeouts
scope: internal/prose/**,internal/mcpauth/**

# ctx and timeouts on lib I/O

- `internal/prose/runtime.go:56` `exec.Command("nvidia-smi")`: pass ctx into `pickVariant`, use `CommandContext` with a timeout.
- `internal/prose/install.go:74` `http.DefaultClient`: use a bounded client.
- `internal/mcpauth/handler.go:47` `context.Background()` in `saving`: take the caller's ctx.
- `internal/prose/install.go:64` prints to `os.Stderr`: take an `io.Writer`.

Test first where a seam exists (stub server that hangs, fake command). Acceptance: `make check`.
