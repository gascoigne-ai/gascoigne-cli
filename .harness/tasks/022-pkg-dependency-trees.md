status: todo
# feat(pkg): dependency tree dialects

Only `mvn dependency:tree` has a dialect (`pkg/mvndeps`). Large trees from `npm ls --all`,
`pnpm list|why`, `yarn why`, `cargo tree`, `go mod graph`, `pipdeptree`, `uv tree`,
`poetry show --tree`, `gradle dependencies`, `composer show --tree` are not handled.

Scope: `pkg/tree` engine: collapse `deduped`/`(*)` subtrees, fold identical subtrees, cap depth, keep
`query` matches. Specs in `detect/pkg.go`, branch in `optimizer/pkgoptimizer`. Share tree folding with
k8s/task code where possible. A depth cut can hide a path: rely on `query` and `raw`.

Done when: `make check` and `make eval` pass, 60%+ saved on large trees; architecture doc updated in the
same commit.
