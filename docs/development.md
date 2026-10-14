# Development

Go 1.27, module `github.com/gascoigne-ai/gascoigne-cli`. Builds stay `CGO_ENABLED=0`.

```sh
make build      # bin/gascoigne (make install → ~/.local/bin)
make check      # gofmt check, go vet, import boundaries, tests: must pass before a change is done
make eval       # token savings on testdata/eval fixtures
make cover      # test coverage
```

## Tests

- Same package as the code, table-driven, `t.Run` for named cases.
- Fakes over mocks: small structs implementing a contract.
- Temporary files via `t.TempDir()`; a ledger via `ledger.Config.Path` in a temp dir.
- Bugs get a failing test first, then the fix.

## Eval

```sh
make eval
bin/gascoigne eval -show rg          # print optimized output of matching cases
bin/gascoigne eval -update           # rewrite goldens after an intended change
```

`make eval` runs every case in `testdata/eval/cases.yaml` through the `cli` pipeline with a
throwaway ledger. Each case checks:

| Check | Meaning |
|---|---|
| `expect_type` | detection produced this type |
| `keep` | these substrings survived |
| `min_saved` | at least this % saved |
| golden | output equals `testdata/eval/golden/<name>.out` |
| raw ok | the original comes back byte-exact from the ledger |

Any failing check is a blocker. Rules for optimizer changes: savings must not regress and
`raw ok` must never fail. When output changes on purpose, review the golden diff, run
`eval -update`, and commit the goldens with the code.

Table columns: `type` is the block types after optimization, `route` the chain that ran (`-`
means a filter bypassed it, e.g. `minsize`).

## CI and releases

GitHub Actions (`.github/workflows/`): `ci.yml` runs `make check`, `make eval` and a pure-Go
(`CGO_ENABLED=0`) build for linux, macOS and Windows on every push to `master` and every pull
request. `release.yml` runs on a `v*` tag: the tag must equal `v` + `version.Number`, then it
builds `-trimpath` binaries for linux/darwin/windows × amd64/arm64 and publishes them with
`checksums.txt` as a GitHub release whose notes are that version's `CHANGELOG.md` section. To
release: add the section, bump `version.Number`, commit, then
tag `v<number>` and push the tag to GitHub.
The website (`gascoigne-webpage`) tells users about the release: add `releases/<number>/index.html`
and its changelog entry, deploy, and only then change `latest.json` to the new id (a later commit):
gascoigne reads that file hourly and opens the page of a newer release once, so it must not
name a page that is not live yet.

## Dependencies

Standard library first. Approved third-party modules: `go.yaml.in/yaml/v3` (config),
`gorm.io/gorm` with `github.com/ncruces/go-sqlite3/gormlite` (ledger, pure Go),
`golang.org/x/net` (HTML parsing), `github.com/modelcontextprotocol/go-sdk` (MCP). Anything else
needs an explicit decision.

## Ledger schema changes

Gorm `AutoMigrate` runs on start and adds tables, columns and indexes. Renames, drops and type
changes need an explicit migration step.

## Code style, briefly

Readability first. Early returns, small functions, short names in small scopes. Modern Go where
it shortens code (`slices`, `maps`, `cmp.Or`, `min`/`max`, range over ints and functions). Errors
wrapped with context. Comments say why, not what. The full rules for contributors and coding
agents are in [`.harness/`](../.harness/README.md).

## Trying the dashboard while developing

```sh
make build && bin/gascoigne dashboard
bin/gascoigne exec -- git log          # in another terminal: watch the run appear
```
