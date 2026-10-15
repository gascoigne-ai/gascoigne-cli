# Background server (`cmd/gascoigne/server.go`)

**Lifecycle**
- One per user on `$GASCOIGNE_SERVER_ADDR` (default `127.0.0.1:7878`; the port is the lock).
- Started detached (setsid, log in the user cache dir) when `/api/health` does not answer, by
  commands with `common.server` set (`exec`, `serve` for mcp/mcp-proxy, the codex hook's
  `optimize`) and by `wrap`.
- Health reports pid, build (hash of binary path + size + mtime), binary mtime and ledger path.
  An older build is replaced (stop + spawn), a newer one kept, another ledger left alone.
- The server stats its binary every 2 s. A new build seen twice in a row → shut down, close the
  ledger, spawn the new binary (`watchBinary`).
- **Idle exit** (`idle.go`): requests in flight and the last one's end are tracked (`/api/health`
  excluded; an open stream counts). After `-idle`/`GASCOIGNE_SERVER_IDLE` (30m, 0 never) the
  server shuts down. Commands start it again (`ensureServer`); agents' UserPromptSubmit hooks
  (claude, codex; opencode `chat.message`) bring it back before the next API request reaches the
  proxy.
- Commands: `server run|start|stop|status`; `dashboard` = `server start -open` (opens a browser
  unless there is no display; `-open=false`); `GASCOIGNE_SERVER=off` disables it.

**Server mode** (`remote.go`): `exec`, `run` and the codex hook (`common.useServer`) build only
their pipeline's receivers and exporters (`service.Options.Remote`) and POST the payload to
`/api/optimize`.
- There a warm service per config hash (defaults + user file + overrides; receivers and exporters
  stripped; ≤ 8 kept; one payload at a time) runs `Pipeline.Optimize`.
- Same build and ledger path required (409 otherwise). Refused, 409 or a broken server → the full
  pipeline is built locally for that payload (which also starts or replaces the server). Pipeline
  errors (422) are returned.
- Runs with other `GASCOIGNE_*` env (raw, cat mode, ledger) stay local. A spawned server drops the
  spawner's `GASCOIGNE_*` env except ledger/config/addr.
- Package-level patterns use `lazyre` (`TestPatternsCompile` runs `lazyre.Check`), so a served
  run never compiles them.

**Reset**: `gascoigne reset` / `POST /api/reset` → `ledger.Reset`: events + steps by filters
(flags only, never env); `-all` also raw and failures, with no filters.
