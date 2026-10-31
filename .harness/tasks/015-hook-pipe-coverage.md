status: todo

The Claude PreToolUse hook wraps a piped command only when the last stage is `head` or `tail`
(`cmd/gascoigne/hook.go:209-245`). `cmd | grep x`, `| jq .`, `| sort | uniq -c`, `| wc -l` are left
alone: replay wrapped 93 of 306 commands (`architecture/hooks.md:131`). This caps the reach of every
optimizer.

Scope: append `| gascoigne -attr cmd='<last stage>'` at the end of the pipeline instead of wrapping the
first stage, so a later `grep` still sees raw text. Cover quoting and `hasShellOperator` edge cases;
extend the replay corpus.

Done when: replay shows a higher wrapped share with no changed command semantics (tests compare
exit status and stdout of the unwrapped part); `make check` and `make eval` pass;
`.harness/architecture/hooks.md` updated in the same commit.
