status: todo
kind: chore
commit: refactor(cmd): move the TOML editor and shell rewriter to their own files
scope: cmd/gascoigne/proxy.go, cmd/gascoigne/hook.go, cmd/gascoigne/codextoml.go, cmd/gascoigne/hookwrap.go, .harness/architecture/agents.md, .harness/architecture/hooks.md, .harness/architecture/mcp.md

# Separate unrelated code in proxy.go and hook.go

- `proxy.go` :443-:625 (`proxyCodexTOML`, `setTOMLKeys`, `tomlKey`, `valueLen`, `tomlString`,
  `tomlStrings`, `tomlQuote`, `tomlArray`) is a hand-written TOML editor, unrelated to JSON server proxying.
  Move to `codextoml.go`, mirroring `jsonedit.go`.
- `hook.go` :218-:500 (`wrapCommand`, `wrapPipeline`, `hasShellOperator`, `splitPipe`, `passthrough`,
  `watched`, `streams`, tables `quiet` and `repl`) is a shell rewriter: move to `hookwrap.go`.
  `transcriptEnd` and `transcriptModel` go to a transcript file.

Pure moves: no logic change. Update file names in `agents.md` (proxy), `hooks.md` (3 references), `mcp.md`
(cites `proxy.go`).

Acceptance: `git diff -M` shows moves only; `make check` passes.
