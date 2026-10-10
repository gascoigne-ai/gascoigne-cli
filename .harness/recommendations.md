# Recommendations

Direction for upcoming work. Rules are in `rules.md`; this is advice.

## Optimizer ideas (not built yet)
None queued; dialect tuning is in task 008.

New tools of an existing family are dialects, not optimizers (skill `add-component`).
Dialects that save little are tracked in task 008.

Shared primitives go to `compress/` so engines and composite families can reuse them.
Keep optimizers deterministic and idempotent (running twice changes nothing more).

## Receivers
- Agent integration: `gascoigne wrap`/`hook` cover claude, codex, pi and cursor. Next: agents
  whose hooks can rewrite shell input too (gemini `BeforeTool` `tool_input`, opencode plugins).
- `httpproxy` (task 005): `net/http/httputil.ReverseProxy` in front of LLM APIs, optimizing
  `tool_result` blocks; never touch system prompts or the last user message by default.
  Header `X-Gascoigne-Raw: 1` → bypass. `GET /raw/<id>` serves originals. Streaming passes through.

## Exporters / extensions
- `upstream` exporter: forwards rewritten request for `httpproxy`.

## Performance notes
JSON optimizer: ~35–70 MB/s (`go test ./optimizer/data/jsonoptimizer -bench .`). Decoding into
`map[string]any` is ~30% of time after removing re-encoding (size estimator, hash dedupe).
SIMD JSON parsing (e.g. a simdjson port; amd64/AVX2 only, needs a scalar fallback) would only
speed that part and still has to build Go values — revisit only if decode dominates real
workloads. `simd/archsimd` still needs GOEXPERIMENT=simd; stats loops are not a bottleneck.

## Practices
- Deterministic output ⇒ prompt-cache friendly. No timestamps or random ids in optimized text.
- Measure before claiming: every optimizer gets eval cases with real-shaped fixtures.
- Keep hint/marker text short; it costs tokens too.
- Never pick a transform for whitespace alone: runs of spaces and newlines are one token each
  (`tokens.Estimate` counts them so); comments are where source tokens are (14–50%).
- Prefer stdlib: `os/exec`, `net/http`, `encoding/json`, `bufio.Scanner`, `regexp` (sparingly; package-level patterns via `lazyre.New`).
