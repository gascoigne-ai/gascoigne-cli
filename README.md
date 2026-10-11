<div align="center">

# 🌘 gascoigne

**Agents read less. Nothing gets lost.**

A token optimizer for coding agents. It shrinks command output, tool results and web pages
before they reach the model, and keeps every original one command away.

`~80% fewer tokens` · `16 families` · `110+ tools & formats` · `claude · codex · gemini · opencode · pi · cursor` · `pure Go, no cgo`

</div>

---

## Start in one line

```sh
go install github.com/lukaszb/gascoigne/cmd/gascoigne@latest && gascoigne wrap claude
```

That's it: Claude Code starts with gascoigne in place for this run. No config files touched;
swap `claude` for `codex`, `gemini`, `opencode` or `pi`. When you like it, `gascoigne install`
keeps it on for the project.

## See it work

```console
$ gascoigne exec -- terraform plan
  # module.queues.aws_sqs_queue.this["q00"] will be created
  + resource "aws_sqs_queue" "this" {
        (known after apply: arn, id, policy, url, …)
      + name                       = "shop-q00"
      + visibility_timeout_seconds = 30
    }

  # module.queues.aws_sqs_queue.this["q01"] will be created (like above)
  # module.queues.aws_sqs_queue.this["q02"] will be created (like above)
  …
Plan: 25 to add, 2 to change, 1 to destroy.
[gascoigne: optimized 10000→1311 tokens · full: gascoigne raw af6b9bf6295f3d8e]
```

The last line is the deal: 87% fewer tokens, and `gascoigne raw af6b9bf6295f3d8e` prints the
untouched original, byte for byte.

### The dashboard

`gascoigne dashboard` opens a live view in your browser: savings per day, per command and per
optimizer, every run as it happens, and any run's original side by side with what the agent read.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/images/dashboard-dark.png">
  <img alt="gascoigne dashboard: tokens saved, savings rate, daily savings, outcomes and a live feed of recent runs" src="docs/images/dashboard-light.png">
</picture>

<sub>The moon's phase is your savings rate: the darker it gets, the more your agents skipped.</sub>

### What it saves

Agents burn context on progress bars, passing tests, `managedFields`, pagination tokens and
license boilerplate. gascoigne knows what each kind of output is for, keeps what an agent acts
on (failures, diffs, errors, anomalies, summaries) and folds the rest.

| Output | Before | After | Saved |
|---|--:|--:|--:|
| `npm install --verbose` | 24,655 | 76 | **99.7%** |
| nginx access log | 73,148 | 436 | **99.4%** |
| `go test -json ./...` | 16,208 | 175 | **98.9%** |
| `kubectl logs` (JSON) | 23,171 | 2,156 | **90.7%** |
| `aws ec2 describe-instances` | 33,835 | 4,301 | **87.3%** |
| `terraform plan` | 10,663 | 1,697 | **84.1%** |
| `kubectl get pods -o yaml` | 37,330 | 9,609 | **74.3%** |
| `git log` | 4,173 | 1,151 | **72.4%** |
| `cat lib/benchmark.rb` (comments) | 6,390 | 2,800 | **56.2%** |

<sub>Tokens (BPE-style estimate, within ~10% of a real tokenizer) on the eval fixtures in <code>testdata/eval</code> · <code>make eval</code> · what they are worth per model: <a href="docs/cost.md">cost</a></sub>

## Install

**With Go** (1.27+), the binary lands in `$(go env GOPATH)/bin`:

```sh
go install github.com/lukaszb/gascoigne/cmd/gascoigne@latest
```

**From a checkout**, into `~/.local/bin` (`PREFIX=…` for elsewhere):

```sh
git clone https://github.com/lukaszb/gascoigne && cd gascoigne && make install
```

Check it: `gascoigne version`. One static binary, no cgo, no services to run: the background
server starts itself when needed and exits after 30 minutes idle.

## Hook up your agent

| Agent | Try it for one run | Keep it on |
|---|---|---|
| Claude Code | `gascoigne wrap claude` | `gascoigne install claude` |
| Codex | `gascoigne wrap codex` | `gascoigne install codex` |
| Gemini CLI | `gascoigne wrap gemini` | `gascoigne install gemini` |
| opencode | `gascoigne wrap opencode` | `gascoigne install opencode` |
| pi | `gascoigne wrap pi` | `gascoigne install pi` |
| Cursor | — | `gascoigne install cursor` |

`install` writes to this project's config; add `-g` for every project, `-proxy` to also route
Claude's API traffic through gascoigne. Arguments after the agent name go to the agent
(`gascoigne wrap claude --continue`). `gascoigne unwrap` removes every trace and keeps a backup.

Shell commands then run through `gascoigne exec`, results from other MCP servers are shortened
after the tool ran, and the LLM API proxy trims what the agent's own tools return (Claude's Read
included, line numbers kept).

## Everyday recipes

**Run anything through it**

```sh
gascoigne exec -- go test ./...              # run a command, print optimized output
kubectl get pods -o yaml | gascoigne run     # or optimize anything on stdin
```

**Get the full output back**

```sh
gascoigne raw af6b9bf6295f3d8e               # the id from the [gascoigne: …] hint
gascoigne exec -raw -- git diff              # skip optimization for one run
GASCOIGNE_RAW=1 make test                    # or for everything in this shell
```

Agents do the same through the MCP `retrieve` tool. A source whose originals keep getting
fetched is marked as badly optimized and passed through untouched next time.

**See what you saved**

```sh
gascoigne gain                               # overview: by agent, command, optimizer, cost
gascoigne gain -by day -since 4w             # trend
gascoigne gain -agent codex -by command      # who saves where
gascoigne dashboard                          # the same, live, in the browser
```

**Read the web cheaply**

```sh
gascoigne mcp                                # MCP server: fetch · compress · retrieve · stats
gascoigne mcp-proxy -- npx some-mcp-server   # shorten another MCP server's results
```

`fetch` turns pages into markdown and compacts API JSON; pass `query` to keep the sections
that matter.

**Tune it without a config file**

```sh
gascoigne exec -set optimizers.grep.max_per_file=50 -- rg TODO
GASCOIGNE_DISABLE=truncate gascoigne wrap claude   # per agent, via env
gascoigne config                                    # print the effective config
```

Every default lives in `configs/default.yaml`; your overrides go to
`~/.config/gascoigne/config.yaml` (written on first run, every option commented out; after
upgrading gascoigne, `gascoigne config -upgrade` moves your settings onto the new template).

**Housekeeping**

```sh
gascoigne flush -older 7d                    # drop stored originals older than a week
gascoigne reset -since 24h                   # clear recent stats
gascoigne forget <source>                    # re-enable a source that was passed through
gascoigne server status                      # the background server: pid, version, ledger
```

**Count and price text**

```sh
gascoigne tokens README.md                   # tokens per model family, and what they cost
```

## Families

Output is detected as `family/dialect` and handled by one engine per family.

| | |
|---|---|
| **test** | go · jest · vitest · bun · mocha · deno · pytest · cargo · rspec · maven · dotnet |
| **build** | go · tsc · eslint · golangci · ruff · mypy · rustc · gcc · clang · javac · dotnet · vite/webpack · docker |
| **pkg** | npm · pnpm · yarn · bun · pip · uv · poetry · go mod · cargo · apt/dnf/brew |
| **task** | nx · turbo · make · just · gradle · compose · CI logs |
| **vcs** | git status/log/diff/blame/branch · gh |
| **search · fs** | grep · rg · ag · git grep · ls · tree · find · du · df · source files |
| **table** | ps · ss · lsof · top · systemctl · docker · any aligned or boxed table |
| **data** | JSON · NDJSON · YAML · CSV · XML · TOML lockfiles |
| **log** | app · journald · syslog · docker · kubectl · access · CloudWatch · GCP · Azure |
| **doc · web** | markdown · rst · man · `--help` · HTML → markdown · HTTP responses |
| **db** | psql · mysql · sqlite · EXPLAIN · redis · mongosh · migrations |
| **k8s · infra · cloud** | kubectl · helm · terraform/tofu · pulumi · ansible · cdk · aws · gcloud · az |

## Guarantees

- **Lossless by retrieval** — originals go to a local sqlite ledger first; every optimized
  output carries its `raw` id. Output that would save less than its own hint costs is left as is.
- **Deterministic** — same input, same output. Prompt caches stay warm, through the LLM proxy too.
- **Edit-safe source** — source files only lose whole lines (comments, outlined bodies); kept
  lines are byte-exact and every gap names its original lines (`// … comments L12-30`).
- **Self-correcting** — sources whose originals agents keep fetching get bypassed next time.
- **Local** — the ledger and dashboard stay on your machine; nothing is sent anywhere.

## How it works

```
input ─▶ receiver ─▶ filters ─▶ router ─▶ optimizers ─▶ exporters ─▶ output
                                                │
                                   ledger: original · gains · failures
```

Pipeline modeled on the OpenTelemetry Collector: typed components built by factories, wired
by YAML (`configs/default.yaml`). Details in [`.harness/architecture.md`](.harness/architecture.md).

## Documentation

Guides and reference live in [`docs/`](docs/README.md): [getting started](docs/getting-started.md),
[agents](docs/agents.md), [MCP](docs/mcp.md), [CLI](docs/cli.md),
[configuration](docs/configuration.md), [families](docs/families.md),
[source files](docs/source-files.md), [ledger, gain and dashboard](docs/ledger.md), [cost](docs/cost.md),
[architecture](docs/architecture.md), [extending](docs/extending.md).

## Develop

```sh
make check    # gofmt, vet, import boundaries, tests
make eval     # token savings and fact retention on fixtures
```

Contributor and agent rules live in [`.harness/`](.harness/README.md).
