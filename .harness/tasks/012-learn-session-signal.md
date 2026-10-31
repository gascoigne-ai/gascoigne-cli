status: todo
# feat(learn): surface the top lesson at session start

`learn.go` already finds `big` outputs (>= 1500 tokens left unchanged, line 29) and reruns, but its value
is unmeasured. `make eval` in this repo is one (8.5k tokens a run).

Scope: surface the top lesson once per session in the SessionStart note, like `mcpNudge`. Then watch the
ledger `big` count for that command to see if it drops.

Done when: test; `make check` passes; before/after `big` count noted in the task; architecture doc
updated in the same commit.
