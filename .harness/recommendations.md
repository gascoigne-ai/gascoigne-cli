# Recommendations

Direction for upcoming work. Rules are in `rules.md`; this is advice.

## Optimizer roadmap
| Optimizer | Technique | Route on |
|---|---|---|
| `cli/git` | status: group by state, drop hint lines; log: one line per commit; diff: trim context | `cmd: "git *"` |
| `cli/gotest` | keep failures + summary, drop passing `=== RUN`/`--- PASS` | `cmd: "go test*"` |
| `cli/ls` | drop perms/owner/date columns, group by extension | `cmd: "ls *"` |
| `jsoncompact` | minify, drop nulls/empties, collapse repeated object shapes into a table | `kind: json` |
| `logdedupe` | collapse repeated lines (`… ×42`), normalize timestamps/ids before compare | `kind: cmd_output` |
| `difftrim` | shrink unified diff context, summarize pure moves/renames | `cmd: "*diff*"` |
| `truncate` | head + tail with elision marker, last resort | default chain tail |

Prefer chains of small optimizers over one big one. Keep each one idempotent.

## Receivers
- `exec`: `gascoigne exec -- git status` runs cmd, sets `cmd` attr, preserves exit code,
  stderr passthrough. Agent hooks rewrite commands to this.
- `mcp`: stdio JSON-RPC (hand-rolled, stdlib `encoding/json`). Tools: `optimize(text, kind, attrs)`,
  `get_raw(id)` (marks failure). Arg `raw: true` → bypass.
- `httpproxy`: `net/http/httputil.ReverseProxy` in front of LLM APIs. Optimize `tool_result`
  blocks in request bodies; never touch system prompts or the last user message by default.
  Header `X-Gascoigne-Raw: 1` → bypass. `GET /raw/<id>` serves originals.
  Streaming responses pass through untouched.

## Exporters / extensions
- `upstream` exporter: forwards rewritten request for `httpproxy`.
- `tokenizer` extension: swap bytes/4 estimate for a real count when accuracy matters.

## Practices
- Deterministic output ⇒ prompt-cache friendly. No timestamps or random ids in optimized text.
- Measure before claiming: every optimizer gets eval cases with real-shaped fixtures.
- Keep hint/marker text short; it costs tokens too.
- Prefer stdlib: `os/exec`, `net/http`, `encoding/json`, `bufio.Scanner`, `regexp` (sparingly).
