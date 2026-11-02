# 🌘 gascoigne

**Agents read less. Nothing gets lost.**

A token optimizer for coding agents. It shrinks command output, tool results and web pages
before they reach the model, and keeps every original one command away.

`70–99% off verbose output` · `16 families` · `150+ tools & formats` · `claude · codex · gemini · opencode · pi · cursor` · `pure Go, no cgo`

---

## Start in one line

```sh
go install github.com/gascoigne-ai/gascoigne-cli/cmd/gascoigne@latest && gascoigne wrap claude
```

Claude Code starts with gascoigne in place for this run. No config files touched; swap `claude`
for `codex`, `gemini`, `opencode` or `pi`. When you like it, `gascoigne install claude` keeps it
on for the project.

Other install options (prebuilt binaries for Linux, macOS and Windows, from a checkout) are in
[getting started](docs/getting-started.md#install).

## See it work

A test hangs and Go prints a 421-line goroutine dump (7,478 tokens). The agent reads this instead:

```console
$ gascoigne exec -- go test ./sync/
[paths: §1=/home/user/src/shop/sync]
panic: test timed out after 10m0s
	running tests:
		TestSyncOrders (10m0s)

goroutine 18 [select, 9 minutes]:
example.com/shop/sync.(*Syncer).Run(0xc0001c4000, {0x71c5a8, 0xc0001b6000})
	§1/syncer.go:74 +0x1b8
example.com/shop/sync.TestSyncOrders(0xc0001a69c0)
	§1/syncer_test.go:31 +0x2f1

goroutine 60 [chan receive, 9 minutes]: +39 more on this stack: 61 62 63 64 65 66 67 68 69 70 …29 more
example.com/shop/sync.(*worker).loop(0xc000200000)
	§1/worker.go:52 +0x7c

… 13 standard library frames omitted
FAIL	example.com/shop/sync	600.021s
[gascoigne: optimized 7478→388 tokens · full: gascoigne raw 17415a92]
```

95% fewer tokens, and the cause is still visible: the test is stuck in `Syncer.Run`, 40 workers
wait on a channel. Long paths became `§1`, identical stacks collapsed, standard-library frames
counted, not printed (output shortened here).

Nothing is thrown away. The original sits in a local ledger, and the agent reads back only what
it needs:

```sh
gascoigne raw 17415a92 -grep 'goroutine 7[0-9] ' -C 1
```

### What it saves

It depends on what your agent runs. gascoigne keeps what an agent acts on (failures, diffs,
errors, anomalies, summaries) and folds the rest, so the noisier the output, the more it removes.

| Output | Before | After | Saved |
|---|--:|--:|--:|
| `npm install --verbose` | 24,655 | 76 | **99.7%** |
| nginx access log | 73,148 | 436 | **99.4%** |
| `go test -json ./...` | 16,208 | 175 | **98.9%** |
| `kubectl logs` (JSON) | 23,171 | 2,156 | **90.7%** |
| `terraform plan` | 10,663 | 1,697 | **84.1%** |
| `git log` | 4,173 | 1,151 | **72.4%** |
| `git diff` of source edits | 3,430 | 2,386 | **30.4%** |
| `rg` search | 3,161 | 2,655 | **16.0%** |
| `cat` of a small Go file | 854 | 854 | **0%** |

Across 268 eval cases the median output shrinks by 69%. These are real-shaped samples, so read
them as an upper bound, not a forecast. In real coding sessions tool output was 4–27% smaller
(median 10%), because most of the work is reading and editing code, and tool output is only part
of what the model reads each turn. `gascoigne gain` shows your own numbers.

<sub>Tokens are a BPE-style estimate, within ~10% of a real tokenizer · fixtures in <code>testdata/eval</code>, <code>make eval</code> · more in <a href="docs/cost.md">cost</a></sub>

## Hook up your agent

| Agent | Try it for one run | Keep it on |
|---|---|---|
| Claude Code | `gascoigne wrap claude` | `gascoigne install claude` |
| Codex | `gascoigne wrap codex` | `gascoigne install codex` |
| Gemini CLI | `gascoigne wrap gemini` | `gascoigne install gemini` |
| opencode | `gascoigne wrap opencode` | `gascoigne install opencode` |
| pi | `gascoigne wrap pi` | `gascoigne install pi` |
| Cursor | — | `gascoigne install cursor` |

`install` writes to this project's config; add `-g` for every project. `gascoigne unwrap`
removes every trace and keeps a backup; `gascoigne uninstall` deletes the ledger, config,
caches and binary. Shell commands then run through `gascoigne exec`, and results from MCP
servers are shortened after the tool ran. Routing Claude's API traffic through the LLM proxy
(`-proxy`, API-key logins only) and per-agent details: [agents](docs/agents.md).

## Everyday commands

```sh
gascoigne exec -- go test ./...              # run a command, print optimized output
kubectl get pods -o yaml | gascoigne run     # or optimize anything on stdin

gascoigne raw <id>                           # full original, id from the [gascoigne: …] hint
gascoigne raw <id> -grep 'FAIL|panic' -C 2   # only matching lines (also -query, -lines A-B)
gascoigne exec -raw -- git diff              # skip optimization for one run

gascoigne show internal/service/pipeline.go:Process   # one function, not the whole file

gascoigne gain                               # what you saved, by agent, command, optimizer
gascoigne dashboard                          # the same, live, in the browser
gascoigne mcp                                # MCP server: fetch · compress · show · retrieve · stats
gascoigne mcp-proxy -- npx some-mcp-server   # shorten another MCP server's results
```

Defaults live in `configs/default.yaml`; your overrides go to `~/.config/gascoigne/config.yaml`
(`gascoigne config` prints the effective config, `-set key=value` overrides one option for a run).
Every command and flag: [CLI reference](docs/cli.md). Retrieval, `gain` and the dashboard:
[ledger](docs/ledger.md). Settings: [configuration](docs/configuration.md).

## What it understands

Output is detected as `family/dialect` and handled by one engine per family: **test**, **build**,
**pkg**, **task**, **vcs**, **search · fs**, **table**, **data**, **log**, **doc · web**, **db** and
**k8s · infra · cloud**. That covers go, pytest, jest, cargo, tsc, eslint, npm, git, gh, kubectl,
terraform, aws and more, plus source files with comments and bodies folded. Stack traces fold
recursion, and a re-run in the same session points at failures you already saw. The full list is
in [families](docs/families.md); source files in [source files](docs/source-files.md).

## Guarantees

- **Lossless by retrieval**: originals go to a local sqlite ledger first; every optimized output
  carries its `raw` id. Output that would save less than its own hint costs is left as is.
- **Deterministic**: same input, same output, so prompt caches stay warm.
- **Edit-safe source**: source files only lose whole lines; kept lines are byte-exact and every
  gap names its original lines.
- **Self-correcting**: sources whose originals agents keep fetching are passed through next time.
- **Local**: ledger and dashboard stay on your machine. The one exception is a read, at most
  every 15 minutes, of a small release file from the project website, which you can turn off
  ([release notice](docs/agents.md#release-notice)).

Experimental, off by default (`gascoigne setup -experimental`): a local language model that drops
low-value words from long prose ([prose compression](docs/prose-model.md)), priced prompts, learned
instructions and a resume snapshot after compaction.

## Documentation

All guides live in [`docs/`](docs/README.md): [getting started](docs/getting-started.md),
[agents](docs/agents.md), [MCP](docs/mcp.md), [CLI](docs/cli.md),
[configuration](docs/configuration.md), [families](docs/families.md),
[ledger, gain and dashboard](docs/ledger.md), [cost](docs/cost.md),
[architecture](docs/architecture.md), [extending](docs/extending.md),
[development](docs/development.md).

## Develop

```sh
make check    # gofmt, vet, import boundaries, tests
make eval     # token savings and fact retention on fixtures
```

Contributor and agent rules live in [`.harness/`](.harness/README.md).

## License

[Apache License 2.0](LICENSE).
