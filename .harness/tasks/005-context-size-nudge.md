status: todo
# feat(hook): context size nudge on prompt submit

About 60% of spend is context re-read: mean context 275k tokens, requests above 400k were 42% of cost.
Backlog item [17] waited for compaction economics, which `cmd/gascoigne/context.go` now has.

Scope: on UserPromptSubmit the hook already reads the transcript tail (`cmd/gascoigne/hook.go:475-502`).
Sum the last `usage` (input + cache_read + cache_write, as `readSession` does). When above a threshold
(about 250k, configurable) add one `additionalContext` line: "context is Nk tokens: finish or /compact".
Signal only, never force (see decisions.md). Pair with the `compaction` experiment in `ab.sh` using
`AB_CLAUDE_ENV`.

Done when: unit test with a transcript fixture; `make check` passes; `.harness/architecture/hooks.md`
and backlog [17] updated in the same commit.
