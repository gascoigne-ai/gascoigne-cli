status: done (follow-ups under Later)

# LLM API proxy

## Why
Hooks reach shell output and MCP results, but not what Claude Code's own tools return. Measured
on local Claude Code transcripts (961k tool-result tokens): Bash 60.5%, **Read 25.0%** (never
touched), WebFetch + WebSearch 2.8%, subagent and plan results ~5%. A hook cannot shorten Read
safely: Claude renumbers the structured content it gets back, so dropped lines shift the
numbers. A proxy edits the text the model receives, where `N\t` prefixes are already rendered,
so kept lines keep their real numbers.

A proxy also works for agents without hooks (anything with a base URL), sees the model of every
request, and could later account real usage from responses.

## Constraints
- Prompt cache: every request resends the whole conversation and a tool result is reread by a
  median of 110 later requests. Any byte that changes in an earlier message re-writes the prefix
  at 1.25× input. So: each tool result is transformed once, cached by content hash, and sent
  identically on every later request; optimizers are deterministic, so a restart recomputes
  the same bytes. System prompt, tools and user/assistant text are never touched.
- Fail closed to the original: any parse or pipeline problem forwards the body unchanged.
- Credentials and headers pass through unchanged (`anthropic-beta` carries the OAuth
  capability: setting only `ANTHROPIC_BASE_URL` keeps a claude.ai login active); responses
  stream through untouched.
- Results gascoigne already optimized (hint present) and edit-type tools are left alone.

## Scope
- `receiver/httpproxyreceiver`: HTTP server (`listen`); Anthropic `/v1/messages` (and
  `count_tokens`, so counts match), OpenAI `/v1/chat/completions` and `/v1/responses`; other
  paths pass through. Per tool result one payload through pipeline `proxy`; attrs from the
  matching tool call (Bash command, Read path, tool name), agent from the User-Agent, model
  from the body. Header `X-Gascoigne-Raw: 1` → forwarded as is.
- No upstream exporter: forwarding is the receiver's job (it holds the request and the
  response stream).
- `gascoigne proxy` runs it; `wrap` starts it detached when not answering.
- Registration: `wrap claude` sets `ANTHROPIC_BASE_URL` (unless the user already points it at a
  gateway); `wrap codex` with an API key adds a model provider; `wrap opencode` sets provider
  base URLs. `install -proxy claude` writes it for good (opt-in: a stopped proxy would stop the
  agent; the SessionStart hook starts it).

## Later
- Real usage accounting from responses (`usage`) into the ledger and cost.
- Gemini API format.
- Optional cross-turn dedupe of identical reads (needs the same cache discipline).
