status: todo
# feat(pkg): package inventory list dialects

Package inventories (`pip list|freeze`, `npm ls --depth=0`, `apt list --installed`, `pacman -Q`,
`brew list --versions`, `gem list`, `go list -m all` text form) are not matched: `pip list` has a
`Package Version` header, but `table/aligned` needs an upper-case header (`detect/table.go:39-62`).

Scope: Row with sniff plus `name version` compaction, in `detect/pkg.go`. Real output fixtures.

Done when: `make check` and `make eval` pass, 20%+ saved; matrix examples present; architecture doc
updated in the same commit.
