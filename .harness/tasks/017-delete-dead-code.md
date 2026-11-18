status: todo
kind: chore
commit: chore: delete code with no callers
scope: cmd/gascoigne/proxy.go,internal/detect/**,internal/lazyre/**,Makefile,.harness/**

# Delete verified-unused code

User approved deleting: `(*proxyResult).add` (`cmd/gascoigne/proxy.go:33`), `detect.lastArg` (`internal/detect/detect.go:180`), `lazyre.ReplaceAllLiteralString` (`internal/lazyre/lazyre.go:75`), and the unused `FamilyVCS/Search/FS/Data/Web` consts in `internal/detect/families.go` (or use them where the literals appear).

Re-grep each across the repo, `gascoigne-sdk` and `gascoigne-jarnam` before deleting. Then run `deadcode ./...` (or `staticcheck -checks U1000`) and report anything new; add it to `make check` only if the output is clean. Acceptance: `make check`.
