# MCP

## Server: `gascoigne mcp`

A stdio MCP server. Register it with any MCP client:

```json
{ "mcpServers": { "gascoigne": { "command": "gascoigne", "args": ["mcp"] } } }
```

`gascoigne wrap claude` and `wrap codex` register it for you.

### Tools

| Tool | Parameters | Returns |
|---|---|---|
| `fetch` | `url` (http/https), `query?`, `raw?` | The response, optimized: HTML as markdown, JSON compressed, headers trimmed. |
| `compress` | `text`, `cmd?`, `file?`, `query?`, `raw?` | The text, optimized. `cmd`/`file` make detection exact. |
| `show` | `file`, `symbol?` | One function, method or type of a source file with line numbers; without `symbol`, its definitions and their line ranges. |
| `retrieve` | `id`, `lines?`, `grep?`, `query?`, `context?` | The original of an optimized result, byte-exact, which marks the source as badly optimized; or only the lines asked for, which does not. |
| `stats` | `by?`, `session?`, `format?` | Savings so far, as plain text (no colors or bars: they cost tokens) or `format: json`. |

Every optimized result ends with:

```
[optimized 37169→4062 tokens · full: retrieve id=7222827457e4a13f]
```

### `query`

`query` says what you are looking for. It steers the lossy parts of optimization:

- **JSON arrays** keep items matching the query (BM25) next to errors, anomalies and both ends.
- **Tables** (`ps`, `kubectl get`, query results) keep matching rows.
- **Long pages** (markdown over `doc.max_tokens`, 4000 by default) keep the intro, then the
  sections that match the query well, then the rest in page order while they fit. Dropped
  sections leave their top-level headings and `… N lines omitted`. Versions are terms, so
  `go1.27` matches `go1.27.2` instead of every line that mentions `go1`.

Example: the Go release history page is 37k tokens of HTML. With
`query: "go1.27 security fixes"` it comes back at about 4k tokens with the go1.27 section in
full and every other release as a heading.

### Limits

| Setting | Default | Meaning |
|---|---|---|
| `receivers.mcp.fetch_timeout` | `20s` | Time limit for one fetch. |
| `receivers.mcp.fetch_max_bytes` | 5 MiB | How much of a response body is read. |

Only `http` and `https` URLs are fetched.

### Sessions

Events from the MCP server are tagged with a session: `-session` if given, else the MCP
transport session id, else one id per server run. The agent defaults to the client's name
from `clientInfo`. `stats` reports the calling session by default; `session: "all"` reports
every session.

## Proxy: `gascoigne mcp-proxy`

```sh
gascoigne mcp-proxy -- npx -y some-mcp-server --flag
```

Starts the downstream server over stdio, mirrors its tools, and optimizes their text results
on the way back. It adds one tool, `gascoigne_retrieve` (name configurable with
`receivers.mcp_proxy.retrieve`), that returns originals. Non-text content (images,
resources) passes through untouched.

Tool schemas are compacted on the way (`compact_tools`): titles that restate a property name and
`$schema`/`$id` markers go, every constraint stays. A server with many tools can hide them behind
two (`max_tools: 40`): `search_tools` finds tools by what they do and returns their schemas,
`call_tool` runs one. Use it for agents without native tool search; Claude Code has its own.

Register the proxy in place of the original server:

```json
{ "mcpServers": { "issues": { "command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "some-mcp-server"] } } }
```

## Why MCP pipelines have no exporters

On stdio, stdout is the protocol channel. Results go back to the client as tool results, so
the `mcp` and `mcp-proxy` pipelines have receivers and filters but no exporters.
