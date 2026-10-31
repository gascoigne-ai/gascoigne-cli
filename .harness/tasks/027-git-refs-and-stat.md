status: todo

`detect/vcs.go:17-28` has no `git reflog`, `stash list`, `tag`, `worktree list`, `remote -v`
(fetch/push duplicate lines), `shortlog`, `for-each-ref`, `submodule status`. A bare `git diff --stat`
(no `diff --git` lines) keeps unscaled stat bars (`diff.go:40-65`), while `statLine` in `log.go:73-88`
already handles them under commit headers.

Scope: add commands at `detect/vcs.go:16` and cases in `gitoptimizer/git.go:25-42`; shorten shas to 7,
fold `checkout: moving` runs, merge remote fetch/push pairs. One group per commit. Real output fixtures.

Done when: `make check` and `make eval` pass; matrix examples present; architecture doc updated in the
same commit.
