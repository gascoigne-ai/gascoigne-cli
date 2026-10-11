# Recommendations

Direction, backlog and lessons. Rules are in `rules.md`; this is advice. Work for the
autonomous loop goes in `tasks/` (one file per task, `status: todo`); finished tasks are deleted,
their record is git history.

## Open
- Real fixtures to replace synthetic ones once tools are at hand: terraform/tofu, pulumi, cdk,
  helm diff, kubectl, aws/gcloud/az, mysql, gh (authenticated). Per change: case savings up,
  `keep` strings intact, goldens reviewed.
- LLM proxy: real usage accounting from responses (`usage`) into the ledger and cost; Gemini API format.
- Agents whose hooks can rewrite shell input (gemini `BeforeTool` `tool_input`, opencode plugins).
- Log engine gap markers: say what a repeated trace's gap holds (`as above`), not just `omitted`.

## Tried and dropped
Measured; don't retry without new data.
- Generic multi-line repeat folding: the log engine already folds repeated traces; after the
  engines +1.6 points on a repeated-trace fixture, before them it broke test/task/log goldens.
- Replacing stale/superseded file reads in the proxy: reads are ~4% of context and the 1h
  prompt-cache TTL leaves ~0.2% of requests cold, so rewriting on a cold cache saves ~0.2% of
  input cost; on warm requests it busts the cache. Unedited re-reads are ~0 (proxy fold covers
  them). Re-measure for 5-minute-TTL agents (codex).
- Model-based text compression, rewriting prior reasoning: nondeterministic or lossy; signed
  thinking blocks cannot change.
- SIMD JSON parsing: decode is ~30% of jsonx time (35–70 MB/s) and Go values must still be
  built; revisit only if decode dominates real workloads.

## Practices
- New tools of an existing family are dialects, not optimizers (skill `add-component`); shared
  primitives go to `compress/`.
- Deterministic, idempotent output ⇒ prompt-cache friendly: no timestamps or random ids.
- Measure before claiming: every optimizer gets eval cases with real-shaped fixtures.
- Keep hint/marker text short; it costs tokens too.
- Never transform for whitespace alone (runs of spaces/newlines are one token each);
  comments are where source tokens are (14–50%).
- Prefer stdlib; package-level regexps via `lazyre.New`, sparingly.
