# Contributing to gascoigne

Thanks for helping. This page is the short version; the full rules live in
[`.harness/`](.harness/README.md) and apply to everyone, human or agent.

## Before you start

- For anything bigger than a fix, open an issue first and say what you want to change and why.
- Small fixes, new dialects and new fixtures can go straight to a pull request.
- Go 1.27. Builds stay pure Go: `CGO_ENABLED=0`, no cgo drivers.

```sh
git clone https://github.com/gascoigne-ai/gascoigne-cli && cd gascoigne-cli
make build    # bin/gascoigne
make check    # gofmt, vet, import boundaries, tests
make eval     # token savings and fact retention on fixtures
```

## The rules that get a change rejected

1. **Never lose information.** Optimized output must stay reversible through the ledger; bypassed
   output stays byte-exact. Optimizers never write to the ledger themselves, core does.
2. **Deterministic.** Same input, same output: prompt caches and the eval depend on it.
3. **Extend, don't special-case.** Add a component (receiver, filter, optimizer, exporter,
   extension) plus config. Never branch on a component inside core.
4. **Core needs approval.** `component/`, `payload/`, `tokens/` and `internal/{config,router,service}`
   are API: discuss in an issue before changing them. Component packages never import the engine
   or each other (`make imports` enforces it).
5. **No new dependencies** unless a maintainer agrees first. Stdlib first; the approved list is in
   [`.harness/rules.md`](.harness/rules.md).
6. **Real fixtures only.** Test data is captured tool output, never invented samples. No secrets,
   real user data or home paths in fixtures, code, docs or commits.
7. **No names of other token-saving tools** in code, docs or commits. Describe the technique
   instead ("command output filtering"). Dataset and license notices are the exception.

## Tests come first

- **Bugs:** write a failing regression test first, see it fail for the reported reason, then fix.
  Put it beside the code, named for the behavior, with a comment on what went wrong. A bug you
  cannot reproduce in a test is not understood yet.
- **New behavior** ships with tests: same package, table-driven, `t.Run` for cases, fakes over
  mocks, `t.TempDir()` for files. Tests that touch config or state use `sandbox(t)`.
- Never skip or delete a failing test, or loosen an assertion to make it pass.
- `make check` must pass before you open the pull request. Say in the description that you ran it.

## Changing an optimizer

Savings are the product, so they are measured.

- Run `make eval`. Savings must not regress, and `raw ok` must never be `NO`.
- A new optimizer is a new package, a registry line, a route in `configs/default.yaml` and an
  eval case (fixture in `testdata/eval/`, expectations in `cases.yaml`).
- Output changed on purpose? Review the golden diff, run `bin/gascoigne eval -update`, and commit
  the goldens with the code.
- Anything that shrinks MCP results also runs `SUITES=mcp make eval`.
- Walkthrough for a new dialect or component: [extending](docs/extending.md).

## Code style

Readability beats cleverness, performance and DRY.

- gofmt, early returns, small functions, short names in small scopes.
- Wrap errors with context (`fmt.Errorf("load %s: %w", path, err)`); don't log and return the same error.
- Comments say why, never what. Doc comment on exported identifiers. No commented-out code.
- Table-driven designs over `switch` sprawl. No speculative options or abstractions.

Full guide: [`.harness/code-style.md`](.harness/code-style.md). Dashboard changes also follow
[`.harness/visual-style.md`](.harness/visual-style.md).

## Docs

Behavior that a doc describes changes in the same commit as the doc: the matching page in
[`docs/`](docs/README.md) and the section of [`.harness/architecture.md`](.harness/architecture.md).
Diagrams are Mermaid, not images or ASCII art.

## Commits and pull requests

Subject: `type(scope): what changed, lowercase, no full stop`, at most 72 characters. Types:
`feat`, `fix`, `docs`, `test`, `refactor`, `build`, `ci`, `chore`, `release`. Drop the scope when
the change spans everything. Add a short body (wrapped at 72) that says why: the problem, the
behavior before, what changes now.

```text
fix(grep): keep context lines around the last match

The last match lost its trailing context when the file ended without
a newline, so agents saw a cut-off hunk.
```

- One focused change per commit; don't mix a refactor with a feature, don't reformat code you
  didn't change.
- Look at `git log -12` and copy the scopes in use.
- Pull request: what and why, which tests cover it, and the `make eval` result when an optimizer
  changed.

## License

By contributing you agree that your work is licensed under the [Apache License 2.0](LICENSE).
