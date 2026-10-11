# gascoigne documentation

gascoigne is a token optimizer for LLM agents. It sits between an agent and what the agent
reads (shell output, MCP tool results, web pages), shrinks it, and keeps every original in a
local ledger so nothing is ever lost.

| Page | Read it when you want to |
|---|---|
| [Getting started](getting-started.md) | install, run your first command, see savings |
| [Agents](agents.md) | use gascoigne with Claude Code, Codex, Cursor or pi |
| [MCP](mcp.md) | give any MCP client `fetch`, `compress`, `show`, `retrieve`, `stats`, or optimize another MCP server |
| [CLI reference](cli.md) | every command, flag and environment variable |
| [Configuration](configuration.md) | change limits, routes and pipelines; every option of every component |
| [Families and dialects](families.md) | know what happens to test, build, git, log, k8s… output |
| [Source files](source-files.md) | understand what `cat main.go` returns and how to edit around it |
| [What savings are worth](cost.md) | how models count tokens and agents pay for tool output; the cost in `gain` and `tokens` |
| [Ledger, retrieval and gain](ledger.md) | get originals back, read the savings report, watch runs live in the dashboard, reset bad sources |
| [Architecture](architecture.md) | understand the pipeline, components and content detection |
| [Extending](extending.md) | add a dialect, an optimizer or another component |
| [Development](development.md) | build, test, measure savings, keep goldens up to date |

## The idea in one screen

```console
$ gascoigne exec -- go test ./...
--- FAIL: TestTotal (0.00s)
    --- FAIL: TestTotal/negative_guard (0.00s)
    price_test.go:21: Total([1], 5) = -4, want 0
FAIL	example.com/shop/price	0.001s
ok 3 packages in example.com/shop: api, cart, util
52 tests passed
[gascoigne: optimized 1488→132 tokens · full: gascoigne raw 9f1c2e7a]
```

- The agent sees what it acts on: the failure, where it is, what passed.
- The last line says how to get the untouched original: `gascoigne raw <id>`.
- The exit code is the command's own, so scripts and agents behave the same.

## Principles

- **Never lose information.** Originals go to the ledger before optimized output is
  returned. If saving fails, the original is returned instead.
- **Deterministic.** The same input gives the same output, so prompt caches stay warm and
  savings are measurable.
- **Know the content.** Output is detected as a `family/dialect` type (`test/pytest`,
  `k8s/get`, `vcs/git-diff`) and handled by an engine that knows what matters in it.
- **Edit-safe.** Source files only lose whole lines, and every gap names the lines it hides.
- **Self-correcting.** A source whose original an agent had to fetch is passed through
  untouched next time.
