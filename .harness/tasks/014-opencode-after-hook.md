status: todo
# feat(opencode): tool.execute.after hook

The opencode integration has only `tool.execute.before` (`cmd/gascoigne/opencode.go:33-40`), so
unwrapped commands and MCP results are never shortened. Codex already has a post hook. Cursor
`postToolUse` is still unverified (hooks.md): do not build it before verifying.

Scope: add `tool.execute.after` that replaces results of unwrapped commands and MCP tools, reusing the
PostToolUse pipeline path.

Done when: generated plugin test; `make check` passes; `.harness/architecture/hooks.md` updated in the
same commit.
