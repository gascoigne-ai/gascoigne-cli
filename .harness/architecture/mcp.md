# MCP and SDK API

## MCP

**`gascoigne mcp`**, a stdio server with these tools:

| Tool | Does |
|---|---|
| `fetch` | web pages → markdown, API JSON compressed; http/https only, timeout, size cap. The cheap way to read web data |
| `compress` | any text, with optional `cmd`/`file` hints |
| `show {file, symbol?}` | one definition with doc comment and line numbers, or the outline without a symbol. Also `gascoigne show file[:symbol]`; `mcptool.Show` on `internal/compress/symbols` (Go parser, Python indent, C-like braces) |
| `retrieve id` | the original; optional `lines`/`grep`/`query` read a part (not a failure). A full read marks the source as badly optimized |
| `stats` | savings (see Ledger → Reports) |

**`gascoigne mcp-proxy -- <server cmd>`** mirrors a downstream server's tools, optimizes their text
results and adds `gascoigne_retrieve`. Non-text content passes untouched.
- Remote servers: `mcp-proxy -url <url> [-sse|-auto] [-header "Name: value"]...` (or config `url`/`sse`/`headers`)
  connects downstream over streamable HTTP (SSE with `-sse`) instead of starting a command; the agent
  still speaks stdio to the proxy. `install`/`wrap` rewrite entries with a `url` (Claude `type: http|sse`; Cursor and Gemini name
  no type, so the args carry `-auto`: streamable HTTP, then SSE) into that command and `unwrap` restores
  them; `httpUrl`, OAuth and header helpers, and codex remote servers stay direct. The server id hashes the url (`remoteProxyArgs` in
  `cmd/gascoigne/proxy.go`), never the url or header values.
- OAuth: a remote server that answers 401 needs a login, and the proxy handles it itself. It starts
  anyway and serves a stand-in: `Instructions` plus one tool, `gascoigne_login`. At once it runs the
  authorization flow in the background (`mcpauth.Login`: PKCE, dynamic client registration, loopback
  redirect) and shows the authorization page; the tool returns the address and restarts a failed
  flow. The flow runs in the background server (`POST /api/mcp/login`, `loginFlows`), which opens the
  browser itself: the proxy is started by an agent, often with no display, in a sandbox, and stopped when
  the agent likes; without a server the proxy runs the flow in-process (`remoteLogin`). (A login nobody completes times out after 5 minutes and is not retried by itself). When the user is done the proxy
  connects, adds the server's tools and removes the login tool (tools/list_changed). Tokens are kept in
  `<state dir>/mcp-auth.json` (0600, by a hash of the url without query), sent and refreshed by
  `mcpauth.NewHandler` on later starts; a refused login brings the login tool back. Streamable HTTP only,
  and not with a static Authorization header (a 401 then names the header). A login made after start
  records no tool stats for that session. `gascoigne mcp-login <url>` does the same ahead of time, for
  headless machines, and `-logout` forgets it. `install`/`wrap` need no probe: OAuth servers are proxied like any other.
- `compact_tools` drops redundant schema keys.
- `max_tools` > 0 (off by default) hides more tools than that behind `search_tools` (BM25 over
  names and descriptions, top 5 with schemas) and `call_tool`, for agents without native tool
  search.

- Lazy tools: the proxy can offer chosen tools of a server on request only (`WithLazy`, tool names by
  server id, read at start: a session's tool list never changes, so the prompt cache holds). The rest
  stay listed; `search_tools` and `call_tool` reach the lazy ones, and the description of `search_tools`
  names them (sorted, at most 40, then "and N more"), so the agent knows they exist and asks for them.
  No tool is lazy when the server has a tool of those names; `max_tools` (everything lazy) wins over a
  choice; names the server lacks change nothing. On a 30-tool fixture with 25 lazy, the list falls from
  about 5.2k to 1.2k tokens (`TestLazyToolsEval`).
- Stats: when the agent lists tools, the proxy records the definition size of each tool it
  showed (as the agent receives it, schemas compacted; rows flagged lazy hold what the lazy tools would cost) under the downstream server's id and counts every call
  (`ledger` `tool_stats`). The id is the name the server reports plus a short hash of its command line
  (`serverID`), so two configurations of one server binary stay apart; only the hash is kept, since
  arguments can hold keys. It is not the agent's config key.

MCP pipelines have no exporters: stdout is the protocol channel.

## SDK API (`cmd/gascoigne/sdk.go`)

Stable loopback contract for gascoigne-sdk (agent frameworks), mounted in the background server beside
`/api/optimize` (which stays internal: it needs the client's build, ledger and config).

| Route | What |
|---|---|
| `GET /sdk/v1/health` | `{version, proto, proxy}`; `proto` (`sdkProto`) is bumped only by a breaking change, with the path |
| `POST /sdk/v1/optimize` | `{text, tool?, cmd?, url?, query?, session?, agent?, model?}` → `{text, raw_id?, raw_tokens, out_tokens, skipped?}`; runs the `cli` pipeline on the user's config and ledger through `warm` |
| `GET /sdk/v1/raw/{id}` | the original behind `raw_id`; instruction text (`hiddenKey`) is never served |

All routes are `localOnly`. SDKs treat any failure as "return the input unchanged".
