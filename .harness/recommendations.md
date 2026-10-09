# Recommendations

Direction for upcoming work. Rules are in `rules.md`; this is advice.

## Optimizer ideas (not built yet)
| Idea | Technique |
|---|---|
| `cargo`, `pytest`, `mvn/gradle` | same shape as `go`: failures + summary, passing collapsed |
| `docker`/`kubectl logs` | `log` already applies; add pod/container prefix normalization |
| `tsc --watch`, webpack/vite | keep errors, drop module/asset listings |
| `diff` moves/renames | detect moved blocks, summarize pure renames |
| `truncate` | head + tail with marker, last resort at the end of the default chain |

Prefer small optimizers selecting one content type; shared primitives go to `compress/`.
Keep each one deterministic and idempotent (running twice changes nothing more).

## Receivers
- Agent integration: a Claude Code PreToolUse hook rewriting Bash commands to
  `gascoigne exec -- <cmd>` (task 007); MCP config `gascoigne mcp` for web data.
- `httpproxy` (task 005): `net/http/httputil.ReverseProxy` in front of LLM APIs, optimizing
  `tool_result` blocks; never touch system prompts or the last user message by default.
  Header `X-Gascoigne-Raw: 1` → bypass. `GET /raw/<id>` serves originals. Streaming passes through.

## Exporters / extensions
- `upstream` exporter: forwards rewritten request for `httpproxy`.
- `tokenizer` extension: swap bytes/4 estimate for a real count when accuracy matters.

## Practices
- Deterministic output ⇒ prompt-cache friendly. No timestamps or random ids in optimized text.
- Measure before claiming: every optimizer gets eval cases with real-shaped fixtures.
- Keep hint/marker text short; it costs tokens too.
- Prefer stdlib: `os/exec`, `net/http`, `encoding/json`, `bufio.Scanner`, `regexp` (sparingly).
