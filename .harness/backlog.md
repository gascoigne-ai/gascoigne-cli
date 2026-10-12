# Backlog

Open ideas, measured results and dropped ideas. Not loaded into every request: read it before
proposing or retrying an optimization, or picking work that has no task yet. Everything under
"Measured" and "Tried and dropped" was measured: don't retry without new data. Work for the
autonomous loop goes in `tasks/`; the practices every change should follow are in
`recommendations.md`.

## Open
- Real fixtures to replace synthetic ones once tools are at hand: terraform/tofu, pulumi, cdk,
  helm diff, kubectl, aws/gcloud/az, mysql, gh (authenticated). Per change: case savings up,
  `keep` strings intact, goldens reviewed.
- Usage: split 1h vs 5-minute cache writes (Anthropic `cache_creation.ephemeral_1h_input_tokens`)
  so `gain` prices 1h writes right; compare measured input with tokens saved per agent.

- Continuity snapshot (`experimental: [continuity]`): A/B on a task that forces a compaction (tokens
  added at each compaction vs rework avoided) is not run; keep it experimental until it is.

- `learn` (experimental): the two habits are the ones the ledger can show; value on real ledgers is
  unmeasured (do the proposed lines change agent behavior?). Failed-command repeats need the exit code,
  which events do not record.

- Hooks: confirm on a real session that Claude subagent tool calls reach the PreToolUse/PostToolUse
  hooks (`gain -by session` for the subagent's calls); check whether Cursor has a result-replacement
  hook; the coverage table in `architecture.md` marks both as unverified.

- Web pages: article benchmark (`.harness/scripts/html-bench.sh`, 181 pages) at recall 98.3%,
  precision 59%. Misses: mega-menus written as plain link lists (not `<nav>`) fill the doc
  budget before the story (forbes, gov.br pages); precision is lost to the same menus, share
  bars and footers outside `<footer>`. Link-dense blocks could fold like `<nav>`.

## Measured
- Marker audit with exact o200k counts (`tokenizer/`; Claude needs an API key, not yet measured):
  raw id 16→8 hex saves 7 tokens per hint (done); `(×60)`→`×60` and `[↑ …`→`[…` save 2 each but
  only ~0.1% of eval output; `… N lines omitted`, `§1`, `_cols/_rows`, `L12-30` have no cheaper
  form. Re-run with `tokens` and a key before changing Claude-facing markers.

- A/B on OpenRouter (`AB_ENV_FILE`, `~anthropic/claude-sonnet-latest`, 4 tasks × 3, "on" arm), 2026-10-11:
  base 12/12, $0.095/run, 484 output tokens; guide line "Keep your own text brief" 12/12,
  $0.091, 408 output tokens (−16%): kept. Instruction pruning (`prompts -terse 0.3` on CLAUDE.md
  and `.harness/*.md`) removes only 1.3–2.4% (the files hold few articles/fillers) and moved
  nothing (12/12, $0.093): no runtime use. Tasks are easy (all arms 100%); rerun on harder ones.

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
- Trimming edit-tool result echoes in the proxy: Claude Code Edit/Write results average ~165 bytes
  (a one-line confirmation), 1.7% + 1.0% of tool-result bytes over 40 sessions; nothing to trim.
- Folding repeated reminders in the proxy: transcripts hold few `<system-reminder>`s; repeats
  are per-prompt hook context (34.6 KB over 40 sessions, ~330 tokens per session vs ~25k tokens
  of instructions per request), mostly prompt-cache reads, and repeating is what those hooks want.
- SIMD JSON parsing: decode is ~30% of jsonx time (35–70 MB/s) and Go values must still be
  built; revisit only if decode dominates real workloads.

- Guide line "to count or aggregate across many files, write a short script and print only the
  result" (2026-10-12, OpenRouter `~anthropic/claude-sonnet-latest`, tasks `count-optimizer-tests`
  and `count-lazyre-files`, 3 repeats each, "on" arm): 6/6 pass with and without the line. Warm-cache
  runs cost the same ($0.027 / $0.046 per task, 2-3 turns); the mean gap ($0.105 vs $0.069) is one
  cold-cache first run in the base arm. Agents already answer these with one shell command.
  Retry on tasks that need reading dozens of files by hand.

- Proxy `shape_output` (a "be terse" note ending the Anthropic system prompt, 2026-10-12, OpenRouter, 4
  tasks × 3, brevity line of the guide in both arms): 12/12 pass each; output 502 → 423 tokens (−16%)
  but mixed per task and noisy. Dropped: the guide's SessionStart line already asks for brevity (also
  measured, −16%), and editing the system prompt in the proxy touches what the API and the login terms
  treat as the client's own; session-start context does not. `ab.sh` keeps `AB_HTTPPROXY` for other
  proxy settings.
