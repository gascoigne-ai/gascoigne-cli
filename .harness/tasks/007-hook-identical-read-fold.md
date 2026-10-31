status: todo
# feat(hook): fold identical Read results to a pointer

28.8% of Read bytes in local transcripts are byte-identical re-reads of the same path (662 repeat Reads,
1.69 MB). The proxy fold covers this only when the proxy is on; login users get nothing.

Scope: first measure from transcripts how many identical re-reads follow a compaction versus none, and
stop if most follow compaction. Then a PostToolUse pointer-only replacement ("same as the Read of this
file earlier in this session, unchanged"). A pointer has no line-numbering problem. Keep a per-session
state file path -> (sha, ts); reset it on SessionStart `compact|clear|resume` (`cmd/gascoigne/guide.go:71-84`)
so a read after compaction is served whole. Risk: Claude Code can drop old results itself, and a false
pointer costs a re-read turn.

Done when: measurement written into the task or backlog; unit tests incl. reset; `make check` passes;
`.harness/architecture/hooks.md` updated in the same commit.
