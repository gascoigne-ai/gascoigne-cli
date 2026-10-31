status: todo

Small items, split into separate commits (or separate task files if one grows):
- `show` supports only Go, Python, C-like (`internal/compress/symbols/symbols.go:34-40`): add Ruby, Elixir,
  Lua (`def…end`), Markdown headings.
- `find -ls` is not parsed: `lsoptimizer/ls.go:26` `entry` regex anchors on the mode char; allow an optional
  `^\s*\d+\s+\d+\s+` prefix.
- `git log --graph` with the default format is not parsed (`log.go:33-34`); low value.
- `pkg/mvndeps` tail cut: keep `BUILD SUCCESS|FAILURE` and error lines (backlog open item).

Done when: per item `make check` and `make eval` pass with real output fixtures; architecture doc updated
in the same commit.
