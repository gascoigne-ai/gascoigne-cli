status: todo
kind: chore
commit: refactor(optimizer): noise alternations as pcomb rules
scope: optimizer/**,internal/pcomb/**

# Replace long noise regexps with pcomb tables

Convert to named `pcomb` rules with Yes/No examples (see `pkgoptimizer/list.go:18`): `catoptimizer/minify.go:45` (29 alts), `k8soptimizer/k8s.go:120` (24), `infraoptimizer/infra.go:56`, `infraoptimizer/others.go:112` (15), `taskoptimizer/gradle.go:12` (13), `gitoptimizer/git.go:51`, `dboptimizer/plan.go:14`. Exact-word lists (`dataoptimizer/toml.go:18`, `lsoptimizer/paths.go:180`) become sets.

Output must be byte-identical: eval baseline unchanged. Acceptance: `make check`, `make eval`.
