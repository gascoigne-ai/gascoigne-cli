# Families

One engine per family (`optimizer/<family>optimizer`); dialects tune it. Shared logic
lives in public libs so engines and composite families can reuse it. Families are what
`gain -by family` reports; `gain -by dialect` splits them.

## Family engines

| Family | Engine (lib) | Dialects |
|---|---|---|
| `test` | `internal/compress/testreport`: go parsed, others via rules (`pass`, `noise`, `frames`, `keep` for pass-shaped lines that stay; a missing rule never matches). Passes counted, noise and dependency frames dropped, failures and summaries kept. Coverage (`test/coverage`, `covtable.go`): lcov, `go tool cover -func`, coverage.py / pytest-cov tables (`Name Stmts Miss … Cover [Missing]`) and istanbul text tables (jest, nyc, c8, vitest) list only files under `coverage_below` plus the total. A table inside a runner's own output (`pytest --cov`, `jest --coverage`: the runner wins detection) is replaced in place by `CoverageTables`, so failures stay; a table alone (`coverage report`, `nyc report`, cat of a saved report) is `test/coverage` | go, go-json, jest, vitest, bun, mocha, deno, pytest, cargo (also bench, llvm-cov, miri), rspec, maven, dotnet, ctest, gtest, catch, sbt, dub, phpunit, swift, mix, minitest, unittest, tap, playwright, cypress, karma, dart, hspec |
| `build` | `internal/compress/watch`: watch output keeps the last cycle + earlier result lines. `internal/compress/diag`: diagnostics grouped by file, maven `symbol:`/`location:` lines kept with their error, identical ones in a file folded with their other lines (`×3, also at 5, 9`), identical ones across 3+ files folded, code frames capped (annotations kept), per-dialect noise/prefix rules (`Noise` is a `pcomb.Rules` table of named rules with `Yes`/`No` examples checked by `TestNoiseRulesVerify`; `Strip`, `Tag` for a level prefix such as sbt `[error]`), shapes `file:l:c:`, `file(l,c):` with optional column and code (msvc, dmd), rust/elixir header + location, dart, kotlinc; `chain.go` keeps the ends of runs of template instantiation notes (gcc, clang) and include chains and folds a linker's `undefined reference` lines into one symbol list | go, tsc, eslint, golangci, ruff, mypy, rustc, gcc, clang, javac, maven, dotnet, bundler, docker (RUN steps composed), cmake, ninja, meson, bazel, cpplint, msvc, dmd, scala, swift, zig, kotlin, haskell, dart, elixir, php, rubocop, pylint, fmt, lint, and `generic` (3+ `file:line: error` lines of any tool) |
| `pkg` | `internal/compress/pkgnoise`: progress and downloads dropped (mvndeps: also separators and Total time; BUILD SUCCESS/FAILURE and `[ERROR]` lines stay) (`Noise` is a `pcomb.Rules` table, as in `diag`), warnings and installs folded by kind. `internal/compress/tree` (dialects `tree`, `gomodgraph`): glyph trees (npm ls, pnpm list/why, pipdeptree, uv tree, poetry show --tree) parsed by label column; leaf children share one line (wrapped at 110, continuation lines end the previous with a comma); `deduped`/`(*)` repeats dropped (kept as `=name` when they match the query); a label written in full once, later uses lose the version when the tree has one version of the package (legend line on top); a package whose dependencies were written already is a plain name; packages deeper than `tree_depth` (config, default 5) counted as `(+N deeper)`, except paths to query terms; `go mod graph` edges grouped by module. `internal/compress/pkglist` (dialect `list`): package inventories (pip list|freeze, apt list, pacman -Q, brew list, gem list, go list -m all) as `name version` lines: header, rulers, apt suite and `[installed]`, gem `default:` dropped; consecutive packages with one version share a line (`5.87-2: bluez bluez-libs`); pip list 15%, apt list 44%, gem list 41%, pacman -Q 7% (versions already differ per line), go list -m all 0%. A pip header with extra columns (`Latest Type`) stays. `go list -m -json|-f` is not a list cargo tree, gradle dependencies, composer show --tree, yarn why share the glyph shapes but have no real fixture yet, so no command routes to them | npm, pnpm, yarn, bun, pip, uv, poetry, go-mod, cargo, os, composer, gem, mix, pub, swift, nuget, conda, deno, nix, flatpak, docker (pull/push layer progress), mvndeps, tree, gomodgraph, list |
| `task` | sections; ok/cached ones collapse; failing ones via `internal/compress/compose` | nx, turbo, make, just, gradle, compose, ci |
| `vcs` | git engine (see below) | git-status, git-log, git-diff, git-remote, git-blame, git-branch, git-refs, gh-list, gh-view |
| `search` | grep engine: matches grouped by file (with or without line numbers), identical lines in a file merged (`3,9: text`, `2× text`); with a `query`, the per-file cap keeps the best BM25 matches | grep, rg, ag, git-grep |
| `fs` | ls engine: listings, tree, paths by directory (past `max_per_dir` only 3+ files sharing an extension fold to `… N more: .go ×N`; extension-less names stay listed), du largest, df real disks. `find -ls` (inode and block columns before the mode) reads as an ls listing. cat: comments dropped, outlines (see Source files) | ls, tree, find, du, df, code |
| `table` | `internal/compress/table`: parses boxed/aligned/ruled/fixed-width tables (one word per column wins over fixed-width slicing; `findmnt` tree prefixes are glued to the target), hoists constants, cuts cells; rows via `internal/compress/sample` (unhealthy statuses kept) | ps, ss, lsof, top, systemctl, docker, sys, aligned, boxed, eval (`report.go`: a report of tables with a `check` column; passing rows fold to a count, the rest stays) |
| `data` | json: `internal/compress/jsonx` crusher. YAML via jsonx when smaller, CSV via table, XML sibling runs sampled (failures kept), TOML lockfiles one line per entry; text lockfiles (`lock.go`: yarn.lock, go.sum, Gemfile.lock; `lockfiles.go` table: pnpm-lock.yaml, pubspec.lock, mix.lock, Podfile.lock, Package.resolved, bun.lock) as `name@version` / `name version` lists | json, ndjson, yaml, csv, xml, toml, ipynb, lock |
| `log` | log engine (see below) | app, journal, syslog, dmesg, docker, kubectl, access, cloudwatch, gcp, azure |
| `doc` | markdown/rst and man/help (see below) | markdown, rst, man, help (+ text: exact-duplicate runs, then the log template engine when ≥ 20 lines and distinct templates ≤ half of them, templates varying by timestamps and uuids only (numbers and hashes are data rows); no debug drop or line cap) |
| `web` | html: `internal/compress/htmlx` → markdown (see below). http: transport headers and TLS chatter dropped, body by type. aria: accessibility snapshots, runs of like siblings capped. search (`websearchoptimizer`, see below) | html, http, aria, search |
| `db` | results via `internal/compress/table`, EXPLAIN pruned to costly nodes, redis keys folded, mongo docs as JSON rows | psql, mysql, sqlite, explain, redis, mongosh, migrate |
| `k8s` | `internal/compress/yamlx` prune rules (managedFields, last-applied, defaults); lists keep unhealthy items; describe/events/helm; apply (`apply.go`): `unchanged` lines fold to one count by kind, other results/errors stay; rollout: progress lines collapse to the final state | get, manifest, describe, events, helm, apply, rollout |
| `infra` | plans: refresh/progress folded, known-after-apply listed, count/for_each siblings "(like above)". terraform/tofu/terragrunt `init` (providers on one line, modules and status kept), `validate` (boxes dropped, repeated warnings folded with their places), `state list` (addresses by module, instances `[×N]`; own grouping, since addresses are dotted not paths), `output` (plain lists on one line; `tfcmds.go`). ansible: quiet tasks counted, failures and recap kept | terraform, terraform-init, terraform-validate, terraform-state, terraform-output, pulumi, ansible, cdk, helm-diff |
| `cloud` | JSON: pagination/metadata/etags pruned, envelopes unwrapped, items flattened so jsonx hoists leaves. YAML via yamlx; tables; s3 listings summarized | aws, aws-s3, gcp, azure |

## Engine details

**vcs (git engine)**
- Diffs: in-line edits as word diffs (`~[-old-]{+new+}`), only when every pair of a k:k block
  qualifies. Lockfiles and generated files fold to counts.
- Blocks removed and re-added verbatim elsewhere → `-… N lines moved to file:L` (`moves.go`).
- Log keeps `--stat` and name lists; `--graph` columns are stripped first (topology not kept, the original is in the ledger). Blame runs are grouped. Mirrored remote branches are dropped.
- Bare `git diff --stat` (also before a patch) gets the scaled bars cut to their signs (`statEnd`). `git-refs` (reflog, tag, stash list, worktree list, remote -v, shortlog, for-each-ref, submodule status): shas to 7, worktree padding gone, fetch/push lines of one URL merged, runs of 3+ `checkout: moving` entries folded to one line naming the branches (`refs.go`).
- gh lists go through `internal/compress/table`.

**log**
- Lines become templates with counts; errors keep their context.
- Over `max_lines` the cap keeps head, tail, errors and stack traces; `rarity: true` (default false) adds `max_lines/8` more: the entries standing for the fewest input lines (`entry.n`), earlier first among equals.
- `miner: drain` (default `regex`) counts repeats by Drain template clusters (`internal/compress/drain`: length + leading-word tree, similarity threshold, deterministic) so lines differing in a few words fold too; warnings, errors and outliers keep the exact key, debug lines cluster apart. A template varies in at most `drain_max_vars` (2) words, so free-text lines (unit names, messages) stay apart. Lines of one cluster differing in words (not numbers) are variants: the first `drain_variants` (3) keep a line each, later ones fold into one line naming them (`(×N; also a, b, +k)`), so a user or device is never silently hidden.
- Codes stay in the repeat key: a number after `status`, `code`, `exit`, `exit code`, `rc`, `errno`, `signal` (up to 3 digits, `categorical.go`) is not masked, so one `status=503` among `status=200` lines is not folded into `(×N)`.
- `bursts: true` (default false) adds `mostly lines a–b` after ×N when ≥ 80% of a template's ≥ 20 repeats fall in ≤ 25% of the block (`burst.go`, densest window). The first line of every template always stays, so a template that appears late needs no novelty detector (`TestLateTemplateStays`).
- Numeric outliers are kept: ≥ 10× the template's median at a place, for templates of 8+ lines; `outliers: mad` (default `ratio`) also keeps a modified z-score (MAD, 0.6745·(v−median)/MAD, mean absolute deviation when MAD is 0) above 3.5 that is ≥ 1.5× the median.
- Container prefixes (kubectl `--prefix`, compose) are cut before and put back as labels
  (`A+B| `, with a legend).
- `dmesg` (`[ 12.345] ` kernel lines, by command or sniff) uses the same template engine.
- Structured cloud events → lines. Access logs → a traffic summary.

**doc**
- markdown/rst: badges, comments, layout HTML and TOCs dropped; code fences capped.
- Markdown over `max_tokens` keeps whole sections in this order: intro, strong BM25 matches of
  `query` (≥ half the best; versions like `go1.27` are terms), then page order. When whole
  sections use under half the budget, the first section left out keeps its head. Dropped
  sections leave their top-level headings + `… N lines omitted`.
- man/help: furniture and boilerplate dropped, dedented. Help with a `query`: option entries are
  scored by BM25 and those ≥ a third of the best are kept.
- Prose model (optimizer `prose`, after `doc` in the chain, own id so its savings show apart in `gain -by optimizer`; experimental `prose` command and setting, off by default (`prose status`, `/api/experiments` and the dashboard's Experiments panel say if it can run), lossy, original in the ledger): `doc/text` and the
  plain paragraphs of markdown (not headings, tables, quotes, lists, HTML, links, fenced code) go
  to the model (package `prose`, loaded in the optimizing process, so the background server keeps it warm; `CGO_ENABLED=0`: `ortrt` opens the ONNX Runtime library with purego and calls its C API; CUDA on
  Linux/Windows, CoreML on macOS, else CPU), an extractive ModernBERT compressor, int8 weight-only ONNX graph, 274 MB, needs ORT ≥ 1.24 (attribution: `docs/prose-model.md`). The experimental `prose` setting sets `optimizers.prose.enabled` in `configData`, so the config a client sends to the server carries it. Words are scored in
  chunks of ≤350 words and ≤510 tokens; a word stays when any of its tokens scores ≥ `threshold`,
  or matches the hard-keep patterns (numbers, paths, flags, identifiers, negations, connectives,
  inline code, links). Dropped words leave no marker. Model missing, failing, panicking or
  malformed → the lines stay as they were, never an optimizer error (that would discard the
  rule-based savings and mark the source failed); after 3 failures it is given up for the process.
  Models (`internal/prose/models.go`, setting `optimizers.prose.model`, Hugging Face names): `kompress-v2-base` and `ctxprune-small`, each with pinned repo, revision, file hashes and graph output (`final_scores` or `logits`, turned into keep probabilities); each model sits in a directory of its own name under the model root (so files never mix and checksums stay per model) and shares the runtime; installs from 0.5.0 kept kompress-v2-base in the root and need `prose install` again; `internal/prose/bpe` reads both tokenizer kinds (byte-level, and "▁" word starts with byte fallback). Install: `gascoigne prose install [-name kompress-v2-base|ctxprune-small] [-runtime auto|cpu|cuda12|cuda13]` (model + ONNX Runtime, pinned hashes; cuDNN for CUDA is the user's). `warm` loads the model in `Start`; `idle` (5m) unloads it after that long unused, the next paragraph reloads it.

**web (html)**
- The root is the `<main>`/`<article>` with the most visible text, when that is ≥ half the page's
  and it neither holds articles nor is named a comment; else the whole page.
- Link-dense blocks (`dense.go`): mega-menus, tag clouds and "popular" lists written as plain lists of links are
  dropped like `<nav>`: a `ul`/`ol`/`div`/`section`/`dl`/`p`/`span` with at least 6 links, 85% of its text inside
  links and labels of 50 bytes on average, and only when the page keeps 1,500 bytes of text outside such blocks
  (a page that is itself a list of links, or whose links are long titles, keeps them). Benchmark
  (`html-bench.sh`, 181 articles): recall 98.3% → 98.8%, precision 59.3% → 64.5%.

**web (search)**
- Shape of Claude's WebSearch, checked in session transcripts (`toolUseResult`, not captured from a live
  hook): `{query, results: [{content: [{title, url}], tool_use_id}, "summary text", …], durationSeconds}`.
  The model sees a text render of it: `Web search results for query: "Q"`, a `Links: [{…}]` JSON line,
  the summary, `REMINDER: You MUST include the sources…`. There is **no snippet per hit**, only title and URL.
- A PostToolUse hook must return that structure unchanged and Claude renders the links itself, so a compact
  list cannot be produced there: WebSearch is not in the hook matcher. The LLM proxy sees the rendered text
  (typed `web/search` by its first line + a `Links: [` line), so only API-key sessions are covered (a claude.ai
  login stays direct).
- Each `Links:` line (there can be several) becomes `- title: url` lines; a title equal to the host is dropped,
  repeated URLs once. Summary and reminder untouched (the reminder is an instruction). Over `max_links` (12;
  real results have 9–10) the best BM25 matches of title + URL against the `query` stay, earlier (better ranked)
  hits first on ties, output in the engine's order, `- … N more links dropped` last. A `Links:` line that is not
  a JSON list of objects with a `url` stays as written. Measured on a real result: 605 → 557 tokens (the summary
  is the bulk).

## Core optimizers (in default chain order)

1. **`embedjson`** (`optimizer/embedjsonoptimizer`, first): JSON values with record arrays
   inside log/test/build/task/text blocks (`jsonx.Spans`) are crushed in place by jsonx; the text
   around them stays byte-exact.
2. **`trace`** (`optimizer/traceoptimizer`, `internal/compress/trace`, runs before the family engines
   on test/build/task/log/doc blocks): recursion cycles once with a count, Go goroutines on one
   stack once, and runs of at least `min_frames` runtime or framework frames as one count
   (`frames.go`: a table of runtimes, each with a frame regex, a noise regex, an optional detail line
   under a frame, a marker wording and `keepTop` for the throw site of JVM and .NET traces). Rows
   for JVM, Python, Rust, Ruby, .NET, Node/Deno/Bun (`at fn (file:line:col)`; noise `node:`, `ext:`,
   `node_modules/`, `(native)`, `(<anonymous>)`; `keepTop`), PHP, gdb/lldb/sanitizers/valgrind (plus
   ASan's shadow dump) and D. Adding a runtime is a row.
3. *Family engines.*
4. **`dense`** (`optimizer/denseoptimizer`): long space-free lines (minified code, base64,
   data URIs) keep head/tail bytes around `…[N bytes dense]…`, once a block holds `min_total`
   such bytes. Lines of names joined by `|`, `,` or `;` (≥8 fields, ≥80% short words) are kept whole.
   JSON lines and fs/code read from disk (file attr, `detect.ReadsFile`) are untouched.
5. **`paths`** (`optimizer/pathsoptimizer`): long path prefixes repeated in
   test/build/search/log/task blocks → `§1/file.go`, with a legend `[paths: §1=…]` on top of the
   first such block. Greedy by bytes saved, kept only when ≥ `min_saving` tokens, idempotent,
   never fs/data. Legend lines are written, detected and expanded
   by `internal/compress/legend` (also used by `rerun` and the probe).
6. **`truncate`** (`optimizer/truncateoptimizer`): blocks still over `max_tokens` keep
   head/tail token budgets + `max_errors` error lines from the middle, each with `error_context`
   lines after it (got/want); with a `query` attr, the `query_lines` best BM25 matches
   (`sample.Terms`/`sample.BM25`) of the middle stay too; long lines are cut.
   `skip`: fs/code, data, k8s, cloud.

## Shared libraries

- `internal/compress/compose` sniffs a section and calls the test/build/pkg engine; used by `task` and
  docker builds.
- `internal/compress/sample` samples records: errors, anomalies, change points, query matches, ends.
- `internal/compress/probe` measures retention: error lines, artifacts (paths, URLs, hashes, UUIDs) and
  `key=number` pairs of an original that are still readable in the output (regrouping, path
  legends and short hashes count as kept).
  - `gascoigne eval` shows lost counts per case and pins them, with out tokens, in
    `testdata/eval/baseline.tsv`; any change fails until `-update`.
  - `gascoigne probe` runs it over ledger originals vs stored outputs (proxy folds skipped), per
    family + worst runs.

## JSON arrays (`internal/compress/jsonx`)

Used by `optimizer/jsonoptimizer`, cloud, data and web.

1. **Small arrays are never sampled**: < `min_items`, < `min_tokens`, or ≤ `max_items`.
2. **Lossless first**: similar objects become a table (`_cols/_rows`), with constant columns in
   `_same` and shared prefixes/suffixes in `_affix`. Items with nested objects are also tried
   flattened to dotted columns (`jsonx.Flatten`); the smaller table wins. Columns with nulls come last
   and rows end at their last non-null cell (a short row means null tail; valid JSON, no `null,null` runs). Kept when it saves
   ≥ `lossless_min_saving` and fits `lossless_max_tokens`.
3. **Else scored sampling** to ~`max_items`, in priority order:
   - errors, always: error fields, bad statuses, 4xx/5xx, keys or enum values few items have
   - by score: anomalies (> `anomaly_std` σ), change points (numeric jumps, enum flips), BM25
     matches of the `query` attr (`-attr query=...`, MCP `query` param)
   - head/tail shares, then an even sample
   - identical items collapse; gaps become `… N items omitted`
