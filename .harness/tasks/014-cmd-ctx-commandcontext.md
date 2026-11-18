status: todo
kind: chore
commit: fix(cmd): thread ctx into subprocess and handler I/O
scope: cmd/gascoigne/**

# ctx for cmd subprocesses and handlers

`exec.Command` -> `exec.CommandContext` at `agents.go:152,158`, `install.go:163,164`, `unwrap.go:84`, `llmproxy.go:150`, `browser.go:35`. Leave the detached background spawns (`pricing.go:93`, `server.go:316`).

Stop ignoring ctx in `cmdWrap`, `cmdInstall`, `cmdPrompts`; replace `context.Background()` in `setup_http.go:88`, `guide.go:191`, `loginserver.go:89`, `help.go:347` with the caller's ctx.

Tests stay green and are unchanged; add one for a cancelled ctx where cheap. Acceptance: `make check`.
