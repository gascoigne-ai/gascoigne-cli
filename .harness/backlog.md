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

- Web pages: article benchmark (`.harness/scripts/html-bench.sh`, 181 pages) at recall 98.8%, precision 64.5% after
  link-dense blocks are dropped. Remaining misses: share bars and footers outside `<footer>`, and link lists of
  long titles that are chrome; the thresholds are in `compress/htmlx/dense.go`.

- Token-saving ideas from the 2026-10-12 ideation pass (score out of 25 in brackets; tasks 040-054
  hold the top ones). Measure each with `ab.sh`, `replay` or `make eval` before keeping it:
  - Deferred on 2026-10-12 because they need paid A/B runs or can break a flow; each had a task that
    was removed (see git history of `.harness/tasks/`): Read size guard [20] (a deny can break Edit
    flows); Read result replacement [18] (first check whether Claude Code accepts replaced Read
    output); cheaper-model exploration agent [19]; repo map at SessionStart [18]; intent-aware query
    for tool results [18]; page outline first for `fetch` [18] (the agent gets less than the page).
    Retry when an A/B budget exists, cheapest first.
  - Cold-cache history diet [17]: shrink old tool results only on requests whose prefix is rewritten
    anyway (after the TTL, compaction, or a changed system prompt or tool set); warm requests stay
    byte-identical. Measure cold vs warm cost with `replay`.
  - Tool description trimming [16]: cap long MCP tool descriptions with `compress/terse`; tool sets
    are about 35k tokens per request, mostly descriptions.
  - Stronger default truncation [16]: `truncate.max_tokens` 10000 to 4000 where the hint reads the rest.
  - Read line numbers on every 5th line in the proxy [12]: risky for edits, needs an edit-task A/B.
  - `max_image_pixels` on by default [17]: measure on sessions with browser tools.
  - Cache lint for instruction files [16]: `prompts` flags volatile text early in the prefix.
  - 1h vs 5m cache-write split in `gain` [17]: see the Usage item above.
  - Subagent hook coverage [17]: see the Hooks item above; if hooks do not run there, a guide line.
  - Gemini `PreCompress` resume snapshot [14].
  - Deferred MCP tools in `mcp-proxy` [15]: one-line summaries plus a search tool, full schemas on demand.
  - Session fetch cache [16]: the same URL again returns "unchanged since <id>" or a diff.
  - Guide lines [17 each]: never restate tool output; delegate wide exploration to a subagent.
  - Context-size nudge at prompt time [17]: needs the numbers from the compaction economics task first.
  - Exit codes in events [15]: lets `learn` and the hint see repeated failures.
  - Pasted logs, JSON and diffs in user prompts through the proxy pipeline [17].
  - Resume vs fresh session price in `gain` [17]; waste report per session in `replay` [17].
  - Instruction tiering in `prompts` [15]; session budget note [13]; test selection hint [14];
    headless task runner [14]; plugin hook pricing [16] (also covered by the settings audit task).

## Measured
- Cache rewrites from transcripts (`gascoigne context`, 2026-10-12, 30 days, one machine, $437 at list prices):
  rewrites cost about $48 extra (11%): pauses over an hour $15, "changed within the cache lifetime" $17 in
  19 requests of roughly 190k tokens each, compactions $12, pauses of 5-60 minutes $4. The 19 changes are
  the thing to explain (model switches and edited instruction files are the likely ones); which one needs
  request bodies, see task 042. Requests above 400k tokens of context were 42% of the cost.

- Guide line "issue independent tool calls together in one turn" (2026-10-12, registration 3): added to the
  full guide without an A/B. Observation to make after two weeks of real use: requests per day per agent before and
  after (the dashboard and `gain` show requests and context per day; the LLM proxy has no session id, so use per-day request counts
  per agent). Drop the line if nothing moves.

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
- Rerun nudge in the hint (2026-10-12): `learn` finds no `rerun` habit on a month of one real ledger (two
  session/command pairs with three or more shortened runs, none by `learn`'s rule), and the `rerun` optimizer
  already folds the repeated lines. Retry if `learn` shows the habit on other ledgers.
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
