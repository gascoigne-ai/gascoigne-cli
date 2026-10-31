status: todo
# feat(data): text lockfile dialects

`dataoptimizer/lock.go:24-33` knows only go.sum, Gemfile.lock and yarn.lock, and `detect/data.go:36`
globs only TOML locks. Agents `cat` other lockfiles whole.

Scope: one format per commit: `pnpm-lock.yaml`, `pubspec.lock`, `mix.lock`, `Podfile.lock`,
`Package.resolved`, `bun.lock`. Output a `name version` list. Real lockfile fixtures. Consider
`composer.lock`, `Pipfile.lock`, `flake.lock` (JSON, currently crushed by jsonx).

Done when: per format `make check` and `make eval` pass, 60%+ saved; `.harness/architecture/families.md`
data section updated in the same commit.
