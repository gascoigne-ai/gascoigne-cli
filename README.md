<div align="center">

# gascoigne

**Agents read less. Nothing gets lost.**

A token optimizer for LLM agents. It shrinks command output, tool results and web data
before they reach the model, and keeps every original one command away.

`~80% fewer tokens` · `16 families` · `110+ tools & formats` · `pure Go, no cgo`

</div>

---

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

## Why

Agents burn context on progress bars, passing tests, `managedFields`, pagination tokens and
license boilerplate. gascoigne knows what each kind of output is for, keeps what an agent acts
on (failures, diffs, errors, anomalies, summaries) and folds the rest.

| Output | Before | After | Saved |
|---|--:|--:|--:|
| `go test -json ./...` | 11,503 | 124 | **98.9%** |
| `npm install --verbose` | 18,666 | 52 | **99.7%** |
| nginx access log | 34,382 | 215 | **99.4%** |
| `aws ec2 describe-instances` | 39,516 | 2,480 | **93.7%** |
| `kubectl logs` (JSON) | 13,459 | 1,351 | **90.0%** |
| `terraform plan` | 10,000 | 1,311 | **86.9%** |
| `kubectl get pods -o yaml` | 34,682 | 8,273 | **76.1%** |
| `git log` | 3,086 | 845 | **72.6%** |

<sub>Tokens on the eval fixtures in <code>testdata/eval</code> · <code>make eval</code></sub>

## Install

```sh
go install github.com/lukaszb/gascoigne/cmd/gascoigne@latest
```

## Use

```sh
gascoigne exec -- go test ./...          # run a command, print optimized output
kubectl get pods -o yaml | gascoigne run # optimize anything on stdin
gascoigne raw <id>                       # the untouched original, byte-exact
gascoigne exec -raw -- git diff          # skip optimization for one run
gascoigne gain                           # where the savings come from
```

**Agents** — one command registers gascoigne (shell output + MCP server) for a run:

```sh
gascoigne wrap claude                    # also: codex, pi
gascoigne wrap -print cursor             # config to register it for good
```

**MCP** — give any agent a cheap way to read the web and tool output:

```sh
gascoigne mcp                            # fetch · compress · retrieve · stats
gascoigne mcp-proxy -- <server cmd>      # optimize another MCP server's results
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
  output carries its `raw` id.
- **Deterministic** — same input, same output. Prompt caches stay warm.
- **Self-correcting** — sources whose originals agents keep fetching get bypassed next time.
- **Tunable per agent** — `-set`, `-only`, `-disable` flags or `GASCOIGNE_*` env, no config file needed.

## How it works

```
input ─▶ receiver ─▶ filters ─▶ router ─▶ optimizers ─▶ exporters ─▶ output
                                                │
                                   ledger: original · gains · failures
```

Pipeline modeled on the OpenTelemetry Collector: typed components built by factories, wired
by YAML (`configs/default.yaml`). Details in [`.harness/architecture.md`](.harness/architecture.md).

## Develop

```sh
make check    # gofmt, vet, import boundaries, tests
make eval     # token savings on fixtures
```

Contributor and agent rules live in [`.harness/`](.harness/README.md).
