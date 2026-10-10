# Families and dialects

Every block of output is detected as a type `family/dialect`: `test/pytest`, `build/tsc`,
`k8s/get`, `vcs/git-diff`. One engine per **family** knows what matters in that kind of output;
**dialects** tune it for one tool. One payload can mix types (a CI log with test and build
sections), and each part meets its own engine.

Savings below are from the eval fixtures in `testdata/eval` (`make eval`), measured as bytes/4.

## How detection works

Evidence, strongest first:

1. **Command.** `exec` and agent hooks know the command line. Patterns are
   `program sub sub -flag`; the most specific match wins. Wrappers and runners are unwrapped:
   `sudo`, `npx`, `uv run`, `poetry run`, `bundle exec`, `pnpm exec`, `python -m`.
2. **File name or MIME type.** `cat deploy.yaml`, MCP `compress` with `file`, `fetch` with a
   `Content-Type`.
3. **Sniffing.** The text itself, specific formats before generic ones.
4. Otherwise `doc/text`.

A generic type from a command, file or MIME type is **refined** when a more specific dialect's
sniffer matches: a `.yaml` file holding Kubernetes manifests becomes `k8s/manifest`, a saved
AWS response becomes `cloud/aws`.

Content gascoigne has no engine for passes through unchanged, and the event records which
types were seen (`gain -by reason`), so gaps show up in the stats.

## test

Passing tests are counted, failures and summaries kept, noise and dependency stack frames
dropped. Go output is parsed; the others follow rules.

| Dialect | Commands |
|---|---|
| `test/go` | `go test` |
| `test/go-json` | `go test -json` |
| `test/jest`, `test/vitest`, `test/bun`, `test/mocha`, `test/deno` | `jest`, `vitest`, `bun test`, `mocha`, `deno test` |
| `test/pytest` | `pytest`, `py.test` |
| `test/cargo` | `cargo test`, `cargo nextest` |
| `test/rspec` | `rspec` |
| `test/maven` | `mvn test`, `mvn verify` |
| `test/dotnet` | `dotnet test` |

```
--- FAIL: TestTotal (0.00s)
    --- FAIL: TestTotal/negative_guard (0.00s)
    price_test.go:21: Total([1], 5) = -4, want 0
FAIL	example.com/shop/price	0.001s
ok 3 packages in example.com/shop: api, cart, util
52 tests passed
```

`go test -json`: 11,503 → 124 tokens (98.9%). `go test -v` with a failure: 1,488 → 132.

## build

Diagnostics grouped by file; identical ones across 3+ files folded into one; code frames
capped at `max_frame` lines with their annotations kept; per-dialect noise and prefixes
removed. Docker builds compose: each `RUN` step's output goes to the matching engine.

| Dialect | Commands |
|---|---|
| `build/go` | `go build`, `go vet`, `go install`, `staticcheck` |
| `build/tsc` | `tsc`, `vue-tsc` |
| `build/eslint` | `eslint`, `next lint` |
| `build/golangci` | `golangci-lint` |
| `build/ruff`, `build/mypy` | `ruff`, `flake8`; `mypy`, `pyright` |
| `build/rustc` | `cargo build/check/clippy`, `rustc` |
| `build/gcc`, `build/clang`, `build/javac` | `gcc`, `g++`, `cc`; `clang`; `javac` |
| `build/maven`, `build/dotnet` | `mvn compile/package/install`; `dotnet build/publish`, `msbuild` |
| `build/bundler` | `vite`, `webpack`, `rollup`, `esbuild`, `next build`, `parcel` |
| `build/docker` | `docker build`, `docker buildx build`, `podman build`, `docker compose build` |

`tsc` with many errors: 1,268 → 381 (70%). webpack warnings: 1,817 → 76 (95.8%).

## pkg

Progress, downloads and resolution chatter dropped; warnings folded by kind; what was added,
removed or failed kept.

| Dialect | Commands |
|---|---|
| `pkg/npm`, `pkg/pnpm`, `pkg/yarn`, `pkg/bun` | `npm install/ci/…`, `pnpm …`, `yarn …`, `bun install/add/…` |
| `pkg/pip`, `pkg/uv`, `pkg/poetry` | `pip install`, `uv pip/sync/add/lock`, `poetry install/add/…` |
| `pkg/go-mod`, `pkg/cargo` | `go mod`, `go get`; `cargo add/update/fetch/install` |
| `pkg/os` | `apt`, `apt-get`, `dnf`, `yum`, `brew`, `pacman`, `apk`, `zypper`, … |

`pip install`: 1,238 → 108 (91.3%). `npm install --verbose`: 18,666 → 52.

## task

Task runners print sections per task. Passing and cached sections collapse; failing ones are
composed: their content goes to the test, build or pkg engine.

| Dialect | Commands |
|---|---|
| `task/make`, `task/just` | `make`, `gmake`; `just` |
| `task/nx`, `task/turbo` | `nx`; `turbo`, `turbo run` |
| `task/gradle` | `gradle`, `gradlew` |
| `task/compose` | `docker compose up` |
| `task/ci` | `gh run view --log`, `--log-failed` |

`docker compose up`: 1,959 → 133 (93.2%). CI log: 2,452 → 195 (92%).

## vcs

| Dialect | Commands | What happens |
|---|---|---|
| `vcs/git-status` | `git status` | hints dropped; staged, unstaged and untracked paths grouped, siblings merged (`internal/service/{pipeline.go,service.go}`) |
| `vcs/git-log` | `git log` | one line per commit (hash, date, author, subject, `+N body lines`); `--stat`/`--name-only`/`--name-status` file lists kept under it, stat bars reduced to their signs (`\| 140 +-`) |
| `vcs/git-diff` | `git diff`, `git show`, `*.diff`, `*.patch` | context cut to `context_lines`, long files capped; small in-line edits as word diffs (`~[-var-]{+const+} minUsagePadding = 25`); lockfiles (`go.sum`, `package-lock.json`, `Cargo.lock`, …) and generated files (`*.pb.go`, `*.min.js`, `Code generated … DO NOT EDIT`) folded to `+N -M lines folded` |
| `vcs/git-blame` | `git blame`, `git annotate` | runs of lines by one commit grouped |
| `vcs/git-branch`, `vcs/git-remote` | `git branch`; `git push/pull/fetch/clone` | mirrored remote branches and transfer progress dropped |
| `vcs/gh-list`, `vcs/gh-view` | `gh pr list`, `gh issue list`, …; `gh pr view` | lists as tables |

`git log`: 3,086 → 845 (72.6%).

## search

Matches grouped by file, each path printed once. Identical lines in a file merge:
`3,9: if err != nil {` with line numbers, `6× if err != nil {` without. Distinct lines per file
capped at `max_per_file`, long lines cut. Output without line numbers (`rg` when not on a
terminal, `grep -r` without `-n`) is recognized when nearly every line starts with a path.

| Dialect | Commands |
|---|---|
| `search/grep` | `grep`, `egrep`, `fgrep`, `ack`, `zgrep` |
| `search/rg` | `rg` |
| `search/ag` | `ag`, `pt` |
| `search/git-grep` | `git grep` |

`rg` without line numbers: 2,279 → 718 (68.5%). `rg -n -i error`: 18,741 → 5,298 (71.7%).

## fs

| Dialect | Commands | What happens |
|---|---|---|
| `fs/ls` | `ls`, `eza`, `exa` | entries packed into lines, sizes only for small listings |
| `fs/tree` | `tree`, `eza --tree` | files per directory capped |
| `fs/find` | `find`, `fd`, `git ls-files`, `rg --files`, `locate` | paths grouped by directory |
| `fs/du` | `du` | largest entries kept |
| `fs/df` | `df` | real disks only (no tmpfs, overlay, loop) |
| `fs/code` | `cat`, `head`, `tail` of source files | comments dropped, outlines; see [Source files](source-files.md) |

## table

Columnar output is parsed (boxed, aligned, ruled, fixed-width). Constant columns are hoisted
into one line, long cells cut, and long tables sampled: unhealthy statuses, anomalies,
query matches and both ends stay.

| Dialect | Commands |
|---|---|
| `table/ps`, `table/top` | `ps`, `top`, `htop` |
| `table/ss`, `table/lsof` | `ss`, `netstat`; `lsof` |
| `table/systemctl` | `systemctl list-units/list-unit-files/list-timers`, `--failed` |
| `table/docker` | `docker ps/images/volume ls/network ls`, `podman ps` |
| `table/aligned`, `table/boxed` | any aligned or box-drawn table (sniffed) |

`ps aux`: 7,409 → 718 (90.3%). `lsof`: 9,620 → 799 (91.7%).

## data

| Dialect | Detected by | What happens |
|---|---|---|
| `data/json` | `*.json`, `application/json`, sniffing | JSON crusher: lossless tables first, then scored sampling |
| `data/ndjson` | `*.jsonl`, `*.ndjson` | records as rows |
| `data/yaml` | `*.yaml`, `yq` | through the crusher when that is smaller |
| `data/csv` | `*.csv`, `*.tsv` | table engine |
| `data/xml` | `*.xml`, `pom.xml`, `*.csproj`, feeds, `xmllint` | runs of repeated siblings sampled, failures kept |
| `data/toml` | `*.toml`, `Cargo.lock`, `poetry.lock`, `uv.lock` | lockfiles one line per package |

The crusher, in order: drop empty values, shorten hashes, cut long strings; turn arrays of
similar objects into `_cols`/`_rows` tables (constant columns in `_same`, shared prefixes and
suffixes in `_affix`; nested objects become dotted columns like `owner.login` when that makes
the table smaller, so a value equal in every item is listed once) when that saves enough;
otherwise sample long arrays to about
`max_items`, keeping errors (error fields, bad statuses, 4xx/5xx, rare keys or values), then
anomalies, change points and query matches, then both ends, then an even sample. Identical
items collapse. Gaps read `… N items omitted`.

## log

Lines that differ only in values (ids, timings, timestamps) fold into one, kept with its
count: `GET /api/cart status=200 dur_ms=20 … (×77)`. Warnings and errors keep `context` lines
around them; DEBUG/TRACE dropped. Structured cloud events become lines. Access logs become a
traffic summary: totals by status class, routes with ids folded (`/api/products/:id ×369
(200 350, 404 19)`), top clients, and the server errors themselves.

| Dialect | Detected by |
|---|---|
| `log/app` | `*.log`, sniffing |
| `log/journal` | `journalctl` |
| `log/syslog` | `syslog`, `messages`, `auth.log`, … |
| `log/docker`, `log/kubectl` | `docker logs`, `docker compose logs`; `kubectl logs`, `stern` |
| `log/access` | `access.log`, sniffing |
| `log/cloudwatch`, `log/gcp`, `log/azure` | `aws logs …`; `gcloud logging read`; `az monitor …` |

nginx access log: 34,382 → 215 (99.4%). `kubectl logs` with JSON lines: 13,459 → 1,351.

## doc

| Dialect | Detected by | What happens |
|---|---|---|
| `doc/markdown` | `*.md`, `glow`, `gh repo view`, HTML after the `html` optimizer | badges, images, layout HTML, comments and TOCs dropped; code blocks capped; over `max_tokens`, whole sections are kept by relevance and page order |
| `doc/rst` | `*.rst` | as markdown |
| `doc/man` | `man` | page furniture and boilerplate sections dropped, dedented |
| `doc/help` | `--help` output (sniffed) | dedented, boilerplate dropped |
| `doc/text` | anything else | runs of repeated lines deduplicated |

**Long pages.** Markdown over `doc.max_tokens` (4000) keeps the intro, then sections that match
the `query` attribute well (at least half the best BM25 score), then the rest in page order
while they fit. Dropped sections keep their heading when it is one of the page's top two
levels, then `… N lines omitted`. Dotted versions (`go1.27`, `v2.3.1`) are search terms with
their prefixes, so `go1.27` matches `go1.27.2`.

## web

| Dialect | Detected by | What happens |
|---|---|---|
| `web/html` | `*.html`, `text/html` | converted to markdown (navigation, scripts, styles and layout dropped), then the `doc` engine |
| `web/http` | `curl -i`/`-v` output (sniffed) | transport headers and TLS chatter dropped, body by its type |

`curl -v https://go.dev`: 12,815 → 3,426 (73.3%). A go.dev blog page: 10,541 → 2,284.

## db

| Dialect | Commands | What happens |
|---|---|---|
| `db/psql`, `db/mysql`, `db/sqlite` | `psql`, `pgcli`; `mysql`, `mariadb`; `sqlite3`, `duckdb` | result tables via the table engine |
| `db/explain` | EXPLAIN output (sniffed) | plan pruned to costly nodes (`plan_min_share`) |
| `db/redis` | `redis-cli`, `valkey-cli` | key lists folded by prefix |
| `db/mongosh` | `mongosh`, `mongo` | documents as JSON rows |
| `db/migrate` | `prisma migrate`, `alembic`, `goose`, `flyway`, `dbmate`, `atlas migrate` | repeated step lines folded with counts, failures kept |

`redis-cli keys`: 4,030 → 62 (98.5%).

## k8s

YAML pruned by rules: `managedFields`, `last-applied-configuration`, defaulted fields, status
noise. Lists keep unhealthy items.

| Dialect | Commands |
|---|---|
| `k8s/get` | `kubectl get`, `kubectl top`, `oc get` |
| `k8s/manifest` | `kubectl get -o yaml/json`, `helm template`, `kustomize build`, manifests in files |
| `k8s/describe`, `k8s/events` | `kubectl describe`; `kubectl get events`, `kubectl events` |
| `k8s/helm` | `helm list/status/history/install/upgrade/rollback` |

`kubectl get pods -o yaml`: 34,682 → 8,273 (76.1%).

## infra

| Dialect | Commands | What happens |
|---|---|---|
| `infra/terraform` | `terraform`/`tofu`/`terragrunt plan/apply/destroy/refresh` | refresh and progress folded; known-after-apply attributes listed once; `count`/`for_each` siblings become "(like above)"; destroys and replacements stay whole |
| `infra/pulumi`, `infra/cdk` | `pulumi up/preview/…`; `cdk diff/deploy/destroy` | as plans |
| `infra/ansible` | `ansible-playbook`, `ansible` | quiet tasks counted, failures and recap kept |
| `infra/helm-diff` | `helm diff` | changes with `diff_context` lines |

`terraform plan`: 10,000 → 1,311 (86.9%).

## cloud

JSON responses: pagination tokens, metadata and etags pruned, envelopes unwrapped, items
flattened so the crusher hoists common values. YAML via the k8s pruner; tables via the table
engine; S3 listings summarized.

| Dialect | Commands |
|---|---|
| `cloud/aws` | `aws`, `sam`, `cdk list` |
| `cloud/aws-s3` | `aws s3 ls`, `s5cmd ls` |
| `cloud/gcp` | `gcloud`, `gsutil`, `bq` |
| `cloud/azure` | `az`, `azd` |

`aws ec2 describe-instances`: 39,516 → 2,480 (93.7%). `aws s3 ls --recursive`: 22,630 → 166.

## Composition

`compress/compose` sniffs a section and hands it to the test, build or pkg engine. The task
family and docker builds use it, so `make test` inside a CI log is treated as test output.

## Path legend (`paths`, every family above that lists files)

Last in the chain, the `paths` optimizer replaces long path prefixes that repeat across test,
build, search, log and task output with short ids, defined once on a legend line:

```
[paths: §1=/home/user/src/web/src]
§1/App.tsx
  82:12 error @typescript-eslint/no-unused-vars 'React' is defined but never used
§1/components/Cart.tsx
```

File names stay visible and the legend gives every full path back. Prefixes are picked greedily
by bytes saved (at least `min_prefix` bytes, repeated, legend entries counted) and applied only
when the payload shrinks by `min_saving` tokens. eslint output: 646→513 tokens; a Go panic trace
after frame folding: 162→123. Source files (`fs`) and data are never touched.
