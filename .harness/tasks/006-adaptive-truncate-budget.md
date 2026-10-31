status: todo
# feat(hook): shrink output budgets when context is large

A token costs more late in a session (large context re-read each turn). Output budgets are static.

Scope: using the context size read from task 005, append `-set optimizers.truncate.max_tokens=4000`
(also `log.max_lines`, `grep.max_files`) in `execPrefix` (`cmd/gascoigne/hook.go:460-472`) when context
is above a threshold. Hook results are produced once, so no cache concern. The hint still points to the
original. Do NOT key the proxy on context size (cache key is content, `results.go:48-63`); at most tier
by the result's position in the request as `fold.go:22-26` does.

Done when: unit test for the prefix; A/B with `ab.sh` shows no task regression; `make check` and
`make eval` pass; `.harness/architecture/hooks.md` updated in the same commit. Depends on 005.
