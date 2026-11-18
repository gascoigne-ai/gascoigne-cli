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

### Acceptance tests (`e2e/`)

`e2e/` runs the compiled binary like a user does and checks what the CLI promises: output, exit
codes, files, and what the ledger remembers between commands. `make check` runs the fast scenarios
through `go test ./...`; `make e2e` runs all, including the `Slow` ones (a real server, eval runs).
Every scenario gets its own HOME, XDG dirs, working dir and a `PATH` holding only `gascoigne`,
`mcpmock` and `emit` (the stand-in tool from `cmd/e2estub`, which also plays `claude`, `codex`,
`gemini`, `opencode` and `pi` when the scenario lists them in `Agents`). The server, proxy and
release check are off, and the server address points at a closed port, so no scenario can reach
a developer's real server.

A scenario is a table row in the file of its area (`dispatch_test.go`, `exec_test.go`, ...):

```go
Scenario{Name: "exec passes the child's exit code through", Steps: []Step{
    {Run: "gascoigne exec -- emit -exit 7", Exit: 7},
}}
```

- `Run` is the command line (quotes work, no pipes); `${work}`, `${home}`, `${root}`, `${bin}`,
  `${record}` and values saved by `Save` expand in it. `Files` creates files in the working dir; `"@testdata/..."` copies
  a repo fixture, so inputs stay real tool output.
- `MCP` turns a step into an MCP session: `Run` starts the server, `Stdout` judges its tool list,
  each `Call` judges one tool result. `Gone` lists paths that must not exist after the step.
  `Server: true` gives the scenario a real background server on a free port, stopped at the end.
- `Stdout`, `Stderr` and `Files` take checks: `Has`, `Lacks`, `Is`, `Empty`, `Like`, `SameAsFile`,
  `ShorterThan`, `JSONField` (`e2e/checks_test.go`). A step fails on the first wrong exit code or
  check, and the message shows the command, both streams and the sandbox path.
- `coverage_test.go` keeps the lists honest: every command in `gascoigne help` must be in
  `docs/cli.md` and have a scenario (or a reason in `notCovered`), and every documented
  `GASCOIGNE_*` variable must be set by a scenario (or a reason in `notExercised`).
- A new area is a new `e2e/<area>_test.go` with `var _ = group("<area>", ...)`; nothing else to wire.
- Run one group: `go test ./e2e -run 'TestScenarios/exec/'`. `GASCOIGNE_E2E_BIN=path` tests a
  prebuilt binary instead of building one.
- A user-visible behavior change needs a scenario. Savings quality stays with `make eval`.

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

`make eval` runs five suites (fixtures, ext, scale, live, ledger). Each starts with a banner: what
it tests, what a result means, how to read the columns. A summary at the end lists ok/FAIL per
suite. The `saved` bar fills 10 cells for 100% and is colored green (60%+), yellow (30%+), red
(below); colors show on a terminal only (`NO_COLOR` turns them off).

Table columns: `type` is the block types after optimization, `route` the chain that ran (`-`
means a filter bypassed it, e.g. `minsize`).

## CI and releases

GitHub Actions (`.github/workflows/`): `ci.yml` runs `make check`, `make eval` and a pure-Go
(`CGO_ENABLED=0`) build for linux, macOS and Windows on every push to `main` and every pull
request. `release.yml` runs on a `v*` tag: the tag must equal `v` + `version.Number`, then it
builds `-trimpath` binaries for linux/darwin/windows × amd64/arm64 and publishes them with
`checksums.txt` as a GitHub release whose notes are that version's `CHANGELOG.md` section. To
release: add the section, bump `version.Number`, commit, then
tag `v<number>` and push the tag to GitHub.
The website (`gascoigne-webpage`) tells users about the release: add `releases/<number>/index.html`
and its changelog entry, deploy, and only then change `latest.json` to the new id (a later commit):
gascoigne reads that file every 15 minutes and opens the page of a newer release once, so it must not
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
