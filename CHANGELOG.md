# Changelog

All notable changes to gascoigne. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/). Each release starts with "In short": what changed for you, in plain
words. The technical details follow.

## [0.10.0] - 2026-11-05

Your own optimizer, your own prices, and a choice between automatic and manual MCP.

### In short

- **Bring your own optimizer.** An external process can shorten output for the commands you route to it (text, JSON
  or RPC); see `docs/external-optimizer.md`. It is accepted from your user config only, never from a project's.
- **Plan or API, found out for you.** Agents on a flat plan stay out of the dollar figures, and you can set prices of
  your own with `gascoigne pricing`.
- **Automatic or manual MCP.** `install -mcp manual` leaves the calls to your agent; `auto` stays the default.
- **Shorter results and guide.** Shortened output ends with `[raw:<id>]` only, and the usage guide is short only.
- **SDKs 0.10.0.** They read totals, runs and costs, and can keep the ledger in memory.

### Added

- **External optimizer.** A process of yours, started per call, receives the output and returns the shortened text
  (`text`, `json` or `rpc` protocol). Configured under `optimizers` in the user config; a project config that names one
  is rejected. Contract and template: `docs/external-optimizer.md`.

- **Automatic or manual.** `install -mcp manual` (or the setup question, or `GASCOIGNE_MCP_MODE`) stops the hooks
  rewriting results and the Grep, Glob and WebFetch deny rules; the agent then calls gascoigne's tools itself.
  `auto`, today's behavior, stays the default.
- **Plan or API, found out for you.** gascoigne looks at how each agent is signed in (an API key or cloud provider, or
  an account sign-in) and leaves agents on a flat plan out of the dollar figures; `billing: auto` is the default. The
  Costs section of `gascoigne setup` holds the choice and the pricing form, with a button to test a price source.
- **No dollars on a flat plan, on the Context page either.** When Claude Code is on a plan, the Context page and
  `gascoigne context` show requests and context sizes instead of dollar figures.
- **Prices of your own, or no dollars.** `gascoigne pricing` sets `-billing auto|api|subscription` (a flat plan: no dollar
  figure in `gain`, the Costs page, `stats` or the SDK), static `-price model=in,out[,cache_read,cache_write]`
  prices, a price sheet `-file`, a `-url` refreshed once per `-ttl` (default 24 h, minimum 1 h), and a `-translate`
  command of yours that turns another format into gascoigne's. A failed refresh keeps the last good prices. The Costs
  page shows where each price comes from; settings are changed on the setup page (behind its one-time link) or in
  the terminal, never on the dashboard, which has no login.
- **A ledger in memory.** `extensions.ledger.memory: true` (or `path: ":memory:"`) keeps the ledger in the process:
  no file, originals capped at `memory_max_bytes`. For embedders such as the SDK.
- SDK: `GET /sdk/v1/stats`, `/recent` and `/costs`, and `inproc.Engine.Stats`, `Recent`, `Costs`, plus the options
  `Ledger` and `Pricing`, so an SDK can read the savings it produced.
- MCP review: servers and tools come most used first, usage suggests which tools to load on request (and advice such
  as `max_tools`), and the Auto-optimize button or `gascoigne mcp-review -optimize` applies the tool suggestions.

### Changed

- Shortened output ends with `[raw:<id>]` instead of a hint with token counts: fewer tokens on every result. The older
  hint still counts as already optimized.
- The usage guide is short only: the hint and path ids, how to read the original, and the ask to keep text brief, readable and condensed. The longer `full` level with
  usage tips is removed; `install -guide` takes `off` or `short` (default), and a saved `full` reads as `short`.

## [0.9.1] - 2026-11-02

The hook no longer crashes on `gh pr checks` and other tables whose rows differ in length.

### In short

- **No more crash on `gh pr checks`.** The hook failed on tables whose rows have different lengths. Your agent now gets the shortened table instead of an error.
- **SDKs 0.9.1.** Python, TypeScript, Go, Java, .NET and Rust, with native demos for every Python and TypeScript framework and Spring AI.

### Fixed

- `gh pr checks` and other tab separated tables with rows of different lengths no longer panic the hook; short rows are
  padded and extra fields get `C<n>` columns.

## [0.9.0] - 2026-11-02

Shorter logs, fewer wasted round trips, MCP savings applied for you, and SDKs that run inside your app with no server.

### In short

- **The SDKs can run inside your app (native mode).** No server and no CLI to install; simpler to deploy and quicker per call. The `inproc` package exposes the same engine in Go.
- **Logs shrink further.** Lines that differ only in a host or user fold together, spikes and bursts are called out, and an error code such as `status=503` is no longer lost among `status=200` lines.
- **Sources your agent keeps re-reading get gentler treatment** instead of being skipped. `gain -refetch` shows how often it happens.
- **MCP review fixes things for you.** It lists tools nobody called this week and can set them to load on request (`mcp-review -optimize` or one button): fewer tool definitions in every request, nothing removed.
- **Try settings on real sessions** with `replay -set`, and get advice on 1-hour cache pauses in `context`.
- **Check two defaults:** a source is bypassed after 2 failures now (was 1), and `optimizers: []` replaces the removed `passthrough` optimizer.

### Added

- `inproc`: public facade that runs the optimizer pipeline inside a process. `configs` exposes the YAML overlay and
  the unknown-settings check.
- Log engine: `miner: drain` folds lines that differ in a few words (host, user) by Drain template clusters;
  `outliers: mad` keeps spikes the 10x median rule misses; `bursts` says where a template's repeats cluster
  (`mostly lines 661-687`); `rarity` keeps the entries standing for the fewest lines over `max_lines`; `passes`
  picks the stages that run before templating. All off or at the old behavior by default except where noted below.
- `truncate`: `query_diverse` picks query lines by maximal marginal relevance, so near-identical matches do not
  fill the budget.
- History: a source fetched raw after an optimized result takes a `gentle` route before it is bypassed
  (`history.gentle_after`), and a whole family goes gentle when too many of its results are fetched raw
  (`history.family_*`). Optimized runs forgive failures (`ledger.recover_after`).
- `gain -refetch`: per family, how often the original was fetched afterwards.
- `mcp-review`: servers and tools sorted by usage, suggestions (tools unused for 7 days to load on request, advice
  for the agent's config) and auto-optimize (`mcp-review -optimize`, the dashboard button; adds lazy tools, never
  removes a choice).
- `replay -set` compares settings on the same transcripts.
- Context: advice on 1-hour cache pauses; skills and subagents no transcript shows are flagged in `prompts`.
- Pipelines list their active optimizers under `optimizers:` in `configs/default.yaml`.

### Changed

- `history.max_failures` is 2 (was 1): with `gentle_after: 1`, the first failure now gets the gentle route and the
  second bypasses. Set `max_failures: 1` for the old behavior.
- Log folding keeps codes apart: `status=503` among `status=200` lines is no longer folded.
- A new release opens the dashboard setup once more at the next session start; Change opens it with a token.
- `optimizers: []` leaves an output as it is; the `passthrough` optimizer is gone (a config naming it needs the
  empty list).
- `ledger` is no longer an option of `mcp`, `mcp_proxy` and `history`.
- `compress.Split` scans once and skips the ANSI regexp on plain text.

### Fixed

- An outlier survives a wide context in log folding; status and exit codes no longer fold into one repeat.

## [0.8.0] - 2026-11-01

Fewer tokens from the session itself, not only from tool output: screenshots are downscaled in the Claude hook,
piped commands are optimized, output budgets tighten when the context grows large, and a failed source no longer
bypasses optimization forever. 30 new dialects and fixes, a costs and context view in the dashboard, and a project
config file.

### In short

- **Screenshots cost less.** Large images the agent reads are scaled down before the model sees them; the original is kept.
- **Piped commands are shortened too**, with the exit status of the pipeline kept.
- **Big contexts get tighter output**, and you get a note about the context size.
- **One bad run no longer blocks a source for good:** it is skipped only after 2 failures within 24 hours.
- **Per-project settings** with a `.gascoigne.yaml` in the repo.
- **30 more kinds of output understood:** Node and Deno stack traces, eslint, cargo and ruff diagnostics, `kubectl apply`, `dmesg`, lockfiles, dependency trees, coverage tables, `docker pull`, git history commands, terraform, web search results.
- **See where the money goes** with new costs and context pages in the dashboard and rerequest columns in `gain`.

### Added

- Claude hook: image Reads above `max_image_pixels` are downscaled, keeping the result's shape and the original
  in the ledger. Pipelines that end in `grep`, `jq`, `sort`, `wc` and the like are optimized, with the exit status
  of the whole pipeline kept.
- Claude hook: a context-size note on prompt submit above a threshold (`$GASCOIGNE_CONTEXT_NUDGE`, `off` to
  disable; nothing is forced), and tighter truncate, log and grep budgets once the context is large.
- MCP results from the PostToolUse hook carry the call's `url` and `query`, as in the LLM proxy, so fetch-like
  results are typed and sampled for the query. `mcp-proxy` can trim tool descriptions (`trim_descriptions`) and
  takes manual `rules`; `fetch` answers a repeated URL with a pointer or a diff.
- opencode: `tool.execute.after` shortens what the rewrite missed.
- `.gascoigne.yaml`, searched from the working directory up to the git root, lays over the user config for
  `exec` and the hooks; each repo gets its own warm pipeline.
- Dialects: Node/Deno/Bun stack frames; JSON diagnostics from cargo, eslint, ruff, golangci-lint, mypy and
  pylint; `kubectl apply|delete|rollout status`; `dmesg`; text lockfiles (pnpm, pubspec, mix, Podfile,
  Package.resolved, bun); dependency trees (`npm ls`, `pnpm why`, `cargo tree`, `pipdeptree`, `uv tree`, gradle
  and more); coverage tables (`coverage report`, `nyc report`, `c8 report`, jest); `docker pull|push`; git `reflog`, `stash list`,
  `tag`, `remote -v` and a bare `diff --stat`; terraform `init`, `state list`, `validate`; web search results;
  system commands (`free`, `vmstat`, `findmnt`, `w`); package inventories (`pip list`,
  `pacman -Q`, `brew list`); the `make eval` table; `show` for more languages.
- `truncate` keeps lines after each error and picks middle lines for the query. Repetitive unknown text is folded
  by the log engine's templates.
- `gain` shows rerequest and wasted tokens per dialect, optimizer and command; `raw` prints a size header for
  large originals; `learn` names its top lesson at session start; per-family Claude token factors price the
  markers.
- Dashboard: costs and context pages with charts, one model table and a detail row per session; `install -window`
  sets Claude's auto-compact window; `context` reports what compactions cost and saved.

### Changed

- `max_image_pixels` is 600000 (was 0, off): screenshots above it are downscaled by the proxy and the hook. Set
  it to 0 to keep images whole.
- `history` bypasses a source after 2 failures (3 for fetch URLs) inside `failure_ttl` (24h); before, one failure
  bypassed it for good.
- `find` and `tree` folds keep directory and package names; `dense` leaves delimited lists alone.
- Test runner rule tables moved to `pcomb`.

### Fixed

- Usage and MCP calls are counted by local day; the day trend keeps today for periods that are not whole days.
- Chained commands are named by the work they do.
- Recorded eval runs stay out of the unpriced agents.

## [0.7.2] - 2026-10-22

Commands with quoted env assignments, like `NODE_OPTIONS="--max-old-space-size=4096" npm run build`, run through the hook again.

### In short

- **Commands with quoted env values work again.** Lines like `DATABASE_URL="postgres://app@localhost/app?sslmode=disable" go test ./...` failed with "executable not found"; they now run and are shortened.
- **No double wrapping** of lines the hook already rewrote, and shell builtins such as `command`, `exec` and `time` are left alone.

### Fixed

- Hook rewrites keep quoted env assignments outside `gascoigne exec`, as in `DATABASE_URL="postgres://app@localhost/app?sslmode=disable" go test ./...` or `NODE_OPTIONS="--max-old-space-size=4096" npm run build`. They failed with "executable not found".
- A line the hook already rewrote is not wrapped a second time when it holds a quoted env value.
- `command`, `exec` and `time` lines stay unwrapped: they are shell builtins or keywords, there is no program for exec to run.

## [0.7.1] - 2026-10-19

The release notice checks for a newer version every 15 minutes, the dashboard gains a costs page, and `make eval`
prints a readable summary.

### In short

- **Costs page** in the dashboard: what your agents spend and what gascoigne saved, over long periods and per session.
- **Hear about updates sooner:** new releases are noticed within 15 minutes.
- **Readable `make eval`:** a banner per suite, savings bars and a summary.

### Added

- Dashboard: a costs page, readable long periods and session filters.
- `make eval` prints a banner per suite, savings bars and a final summary.

### Changed

- The release notice reads `latest.json` every 15 minutes, from agent hooks and `dashboard` instead of the
  background server, and a prompt (not only a session start) opens the page of a newer release.

### Fixed

- The release page is retried until it opens.
- `eval.sh` runs on the bash 3.2 shipped with macOS.
- The fake MCP authorize endpoint (tests) redirects only to loopback, to the fixed callback path.

## [0.7.0] - 2026-10-16

Many more tools are recognised and shortened (build, test and package managers, stack traces, source outlines for
dozens of languages), and `gain` prints a short report by default and keeps its history past a year.

### In short

- **Simpler `gain`.** It prints the essentials by default; `-full` shows everything, and flags filter and export CSV.
- **History that lasts.** Old runs are rolled into daily totals kept forever, so your savings history survives a year.
- **Many more tools recognised.** Build, test and package-manager output of dozens of tools is shortened, stack traces fold runtime frames, and source outlines cover more languages.

### Added

- `gain -csv` for `-by` views and `-failures`; failing sources show when they last failed as "2d ago"; the overview
  header prices the savings (`worth ~$…`) when a cost estimate exists.
- `gain` controls: `-full`, `-show`/`-hide` overview sections, `-top` (default 10 rows per table, 0 all), `-sort`,
  `-min-tokens`. Plain `gain` now prints the essentials (totals, last 7 days, agents, commands, failing-sources warning); `-full` prints every section. Tables are cut at 10 rows with a count of the rest.
- `gain` history survives `events_ttl`: runs pruned from the ledger are first added to daily totals (`daily_stats`,
  `daily_step_stats`), kept forever by default (`ledger.rollup_ttl`). Session and reason groupings only cover stored runs.
- Source outline for Ruby, Crystal, Elixir, Lua, Julia (`end`-closed blocks), Haskell, Elm, OCaml/F#, Nim (declarations
  with indented bodies) and Clojure, Lisp, Scheme, Racket (parenthesis balance); Perl, R, PowerShell, Groovy, Solidity,
  Move, Odin and Vue/Svelte/Astro use the brace outline. Shebangs identify scripts without a file name.
- Build output of CMake, Ninja, Meson, Bazel, clang-tidy/cppcheck, MSVC, dmd/ldc2/dub, sbt/scalac, Swift,
  Zig, Kotlin, GHC/cabal, Dart, rubocop, pylint, formatters and other linters is recognised and grouped
  like compiler output; any unknown tool printing `file:line: error` lines is handled as `build/generic`.
  Dialects can be declared as one `detect.Row` with their own command and content examples.
- Test output of ctest, GoogleTest, Catch2, sbt/ScalaTest, dub, PHPUnit/Pest, swift test, mix test, minitest,
  unittest, node:test/TAP, Playwright, Cypress, Karma, flutter/dart test and cabal/stack: passes are counted,
  failures and summaries kept, dependency frames dropped. `cargo bench`, `llvm-cov`, `tarpaulin` and `miri` use
  the cargo rules.
- Package manager output of Composer, Bundler/gem, mix, pub, SwiftPM, NuGet, conda/mamba, deno, Nix, Flatpak/snap
  and `mvn dependency:*`: progress and download lines dropped, installs folded into lists, failures kept.
- Text lockfiles (`yarn.lock`, `go.sum`, `Gemfile.lock`) shrink to `name@version` lists (`data/lock`). `docker stats`,
  `nerdctl`, `crictl`, `lsblk`, `lsmod`, `w` are recognised as tables; `gh pr diff`, `glab mr diff`, `jj`/`hg` git-style
  diffs and `gh pr checks`, `glab` listings use the git and forge engines.
- Compiler output: runs of C++ template instantiation notes (gcc "required from", clang "in instantiation of … requested
  here") and include chains keep their ends and count the middle; a linker's `undefined reference` lines become one list of symbols; gcc's "required from" lines count as
  notes, not errors.
- Stack traces: runs of runtime and framework frames (JVM/Scala/Kotlin, Python site-packages, Rust backtraces, Ruby
  gems, .NET, PHP vendor, gdb/lldb and sanitizer frames in libc/libstdc++, D runtime) fold into one count
  (`trace.min_frames`); the first frame under an exception line stays, and AddressSanitizer's shadow dump folds.
- Source outline knows D (`.d`, `.di`) and more C++ (`.cxx`, `.hh`, `.hxx`, `.ipp`, `.tpp`, `.inl`, `.cppm`,
  `.ixx`, `.cu`, `.cuh`) and Scala (`.sc`, `.sbt`) extensions; Rust is also recognised without a file name.

- `make eval` runs every suite (fixtures, ext, scale, live, ledger) with one summary; `testdata/eval-scale` adds generated
outputs of 3k to 50k lines with `max_ms` ceilings, and 22 dialects that had no case now have one.

### Changed

- `gain` prints the essentials by default (totals, last 7 days, agents, commands, failing sources); `-full` prints every
section as before, and tables stop at 10 rows (`-top 0` for all).
- New table `daily_stats` / `daily_step_stats` and setting `ledger.rollup_ttl` (default `0s`: forever). Runs older than
`events_ttl` (still a year) are rolled into daily totals instead of being lost; the tables are created on first start,
nothing is migrated.

### Fixed

- Source outline: Rust lifetimes (`&'static str`), nested block comments (Rust, Scala, D), C++ and Rust raw
  strings and digit separators no longer break brace counting; functions inside `mod`, `union`, `package`
  and `extern "C"` blocks are outlined.
- The release notice reads `latest.json` hourly instead of daily, so a release published the day after a
  read is no longer hidden for up to 24 hours.

## [0.6.1] - 2026-10-14

The dashboard tells optimizing apart from rerequesting: when the agent asks for the original after an
optimized result, the run is no longer counted as a saving.

### In short

- **Honest savings.** When the agent asks again for the original, that run counts as a cost, not a saving, so the numbers are net. Old history is converted on first start.
- **Better dashboard:** a per-day chart of optimized versus rerequested runs and a feed you can filter by outcome.
- **Retention you can set:** originals stay 30 days, run statistics a year. Also fixed: `gascoigne tokens` hanging and a git branch crash.

### Added

- Outcome `rerequested`: an optimized run the agent then fetched in full (`gascoigne raw`, MCP retrieve)
  or reran raw is stored as rerequested, not optimized. Runs recorded before are converted once, on the
  first start of the ledger.
- A rerequested run costs the agent the optimized result plus the original, so savings, `gain` and the
  cost section now subtract it.
- Dashboard: an "Optimizing vs rerequesting" chart (gross saved, given back, net, rerequest rate per day;
  `GET /api/impact`), and run feed filters Optimized, Bypassed, Unchanged, Rerequested, Raw (the agent
  asked for the original at once) and Failed. Bypassed has its own color.
- `ledger.events_ttl`: how long runs, API usage and failure history stay.

### Changed

- Retention defaults: originals 30 days (was 24 hours), MCP tool stats one year (was two weeks), runs, API
  usage and failure history one year (were kept forever). `0` keeps any of them forever.

### Fixed

- `gascoigne tokens` with no file and a terminal on stdin waited for input forever; it now says what to do.
- `gascoigne tokens` no longer stalls on long text without spaces (minified or base64 data): the o200k
  count was quadratic. Its downloads and the Claude count request time out after 30 seconds.
- Groups in `gain` were ordered by gross savings, ignoring the rerequest cost.
- `git branch` output with blank lines (several commands run together) crashed the git optimizer.

## [0.6.0] - 2026-10-14

A second prose model you can pick in setup, a progress bar for its download, and an API for agent
framework SDKs. The prose model stays an experiment.

### In short

- **Choose the prose model:** a general one (274 MB) or one tuned for tool output, logs and code (563 MB). Setup asks, downloads and checks it.
- **Progress while downloading;** models install into their own folders, so run `gascoigne prose install` again if you used 0.5.0.
- **For framework developers:** a `/sdk/v1` API for the agent SDKs, a tiny Alpine Docker image, and plainer dashboard labels.

### Added

- A second prose model, `ctxprune-small` (563 MB, trained on tool output, logs, code and web text). Pick it
  with `optimizers.prose.model` or in `gascoigne setup`, which downloads the model you choose and checks
  its files; `gascoigne prose install -name <model>` fetches either. The ONNX Runtime is shared.
- A progress bar while `prose install` and setup download the model and the runtime, when stderr is a
  terminal.
- `/sdk/v1` on the background server: the API the agent framework SDKs use.
- A minimal Alpine `Dockerfile`: a static build, a non-root user, the subcommand as the container command.
- The dashboard shows the live status on every page and records the MCP tool lists daily.

### Changed

- The default prose model is the int8 weight-only graph: 274 MB instead of 601 MB, the same savings,
  identical output on CPU and CUDA.
- Models are named as on Hugging Face (`kompress-v2-base`, `ctxprune-small`) and each installs into a
  directory of its own name, so files and checksums never mix. **Installs from 0.5.0 kept the model in the
  root: run `gascoigne prose install` once more.**
- `make eval-experimental` names the model of each run, so your setup choice does not change its goldens.
- The dashboard's run details use plain labels (Command or tool, Steps tried, Skipped by, Original id), and
  the prose step explains in plain words what it did or why it skipped.
- Maven errors keep their symbol and location lines, and a repeated finding lists every line it occurs at.

### Fixed

- Unpacking the ONNX Runtime archive cannot write outside its directory, whatever entry names or symlink
  targets the archive holds.

## [0.5.0] - 2026-10-13

Prose compression with a local model, and a dashboard that groups its tables. The model is an experiment:
off until you turn it on.

### In short

- **Shorter prose, on your machine.** An experimental local model drops low-value words from long text and markdown; numbers, paths, flags, code and links stay, and the original is kept. No Python, no cgo.
- **Easy to try:** `gascoigne prose install` and `status`; setup downloads the model with its checksums shown.
- **See it in the dashboard:** a -N tag on shortened runs and an Experiments panel.

### Added

- Experimental prose compression (`experimental: [prose]`): a ModernBERT extractive model drops low-value
  words from the plain paragraphs of text and markdown, including converted web pages. Numbers, paths,
  flags, identifiers, negations, code and links stay; tables and aligned output are left alone. It runs
  inside the process that optimizes, on CUDA (Linux, Windows), CoreML (macOS) or the CPU, with no cgo and no
  Python (ONNX Runtime is opened at run time), and the background server keeps it loaded. It has its own
  optimizer id `prose`, so its savings show apart in `gain -by optimizer`.
- `gascoigne prose install` downloads the model and ONNX Runtime for your platform, checks pinned SHA256
  hashes and prints them; `gascoigne prose status` says whether it is ready. `gascoigne setup` runs the
  download when you turn the experiment on and checks the files again later.
- `optimizers.prose.idle` frees the model and its GPU memory after five idle minutes; a damaged download
  fails with a clear error.
- Dashboard: an Experiments panel for the features that are on, a `prose −N` tag on runs the model
  shortened, a prose line in run details and the run's raw id.
- `eval -record` writes an eval run to your ledger and starts the dashboard; `make eval-experimental`
  uses it. A separate eval set, `testdata/eval-experimental`, covers the opt-in features.

### Changed

- The dashboard's tables are tabs in two panels: models and cost (families, models, API requests) and
  breakdown (command, optimizer, content type, not optimized, excluded sources); the tab is remembered.
- `eval` ignores experimental settings, so its numbers do not depend on your choices.
- New dependencies: `github.com/ebitengine/purego` (loads ONNX Runtime without cgo) and `golang.org/x/text`
  (Unicode normalization for the tokenizer). Builds stay `CGO_ENABLED=0`.

### Fixed

- The dashboard page tests never ran outside GOOS=js (the file name ended in `_js`).

## [0.4.1] - 2026-10-13

### In short

- **Update notices work.** The daily check read the website at its old address and never told you about a new version.

### Fixed

- The release notice read `latest.json` from the website's old address (404 since the site moved to
  `gascoigne-ai.github.io`), so no update was ever signalled.

## [0.4.0] - 2026-10-13

Prebuilt binaries on every release.

### In short

- **Download and run:** prebuilt archives for Linux, macOS and Windows (amd64, arm64) with checksums on every release.
- **`gascoigne context`** shows where conversation size and cache rewrites cost you money.
- **Weekly MCP review:** which servers you use and how to switch one off.

### Added

- Linux, macOS and Windows archives (amd64, arm64) with `checksums.txt` attached to each GitHub release,
  with install one-liners that verify the checksum (README, getting started, website).

### Fixed

- The release workflow attaches the binaries to a release created in the web UI instead of failing
  because it already exists, and can be run by hand for an existing tag.
- Links with `data:` and `vbscript:` targets are dropped from fetched pages like `javascript:` ones;
  `after`/`before` in the dashboard events API are parsed without truncation.

## [0.3.1] - 2026-10-12

Where context costs money, and a weekly look at MCP servers. `gascoigne context` and its page show what
conversation size and cache rewrites cost, and a browser page reviews MCP servers.

### In short

- **See where context costs money** with `gascoigne context` and its dashboard page.
- **A weekly look at your MCP servers,** with unused tools offered on request only.
- **Fixes:** HTML source read through a pipe stays source, the release-notice opt-out reaches the background server, and the local API and state files are hardened.

### Added

- `gascoigne context` and a page in the dashboard: where conversation size costs money, what compacting
  saves, cache written again instead of read (by cause), 1-hour and 5-minute cache writes priced apart,
  what hooks, plugins and instructions add, and subagent requests counted apart.
- A weekly MCP review page: per-server use from daily per-tool stats (kept 14 days), where a server is
  configured and how to switch it off, a session-start nudge, and unused tools offered on request only
  (`mcp-proxy`).
- Requests and context size per day in `gain` and the dashboard; hour-long cache writes priced apart
  in the proxy's usage rows.
- The resume snapshot names outputs gascoigne shortened, so agents read them with `gascoigne raw`.
- The guide asks agents to batch independent tool calls.
- Web pages: link-dense blocks such as navigation are dropped.

### Changed

- Command rewriting handles `| head` / `| tail` per command of a list and quoted pipes; multi-line
  commands stay as they are.
- The context and MCP pages follow the dashboard's look, with icon buttons and copyable session ids.

### Fixed

- HTML source read through a pipe stays source instead of becoming a page title.
- The release notice opt-out reaches the background server; the setup page shows the release number.
- The local API and state files are hardened.

## [0.3.0] - 2026-10-12

Setting up and keeping up to date. gascoigne asks, in a browser page or the terminal, before it
changes how your agents are set up, `gascoigne upgrade` brings everything current after a new
version, and `gascoigne doctor` says what is wrong in plain words. The dashboard shows savings by
model and sorts and filters every table.

### In short

- **Setup you control.** A page in your browser (or `gascoigne setup`) asks first; nothing about your agents changes before you answer.
- **Upgrades fix themselves:** `gascoigne upgrade` rewrites stale or lost hooks, rules and commands and moves your config to the new template.
- **Clearer help:** `gascoigne doctor` in plain words; agents found on your machine are offered, not installed.
- **Experiments, opt in,** each with an example and its impact, and a dashboard with savings by model family.

### Added

- A release notice: once a day the background server reads `latest.json` from the project website;
  when a newer release is out, the next session opens its page once. A plain request with nothing
  about you in it; off with `GASCOIGNE_RELEASE_CHECK=off` or in `gascoigne setup`.
- `install -guide off|short|full` (or `$GASCOIGNE_GUIDE`): how much usage text agents get.
- `install -permissions none|local|all`: allow rules for gascoigne's own tools in claude, removed
  exactly by `unwrap`.
- `gascoigne upgrade [-n]`: after installing a new version, one command writes stale or lost hooks,
  rules and commands again, moves `config.yaml` onto the new template (settings kept) and opens the
  page for choices still waiting. `install -autosync on` does the user-level part by itself.
- `gascoigne setup` and a `/setup` page in the dashboard: permissions and hook updates are asked
  once, in the browser (or the terminal), before anything is changed.
- A coding agent found on PATH without a gascoigne install is signaled once (session note and the setup page) and installed only on a yes.
- `gascoigne doctor` (a plain-language report), and `/gascoigne-stats` and `/gascoigne-doctor` commands in claude.
- `gain -oneline`: totals on one line, for a status line.
- Experimental `continuity`: after a compaction or resume, claude gets a short snapshot of its last request, open tasks, edited files and failed commands.
- Experimental `learn`: propose instruction lines from habits that cost context (reruns of shortened output, large unshortened output).
- Dashboard: every table, the recent runs included, sorts by clicking a header and has a filter box.
- Dashboard: savings by model family and by model, in the model's own tokens and dollars, with the official list prices they are estimated from.

### Changed

- The setup page shows what gascoigne found (agents and their state), groups the questions, and says under each option what it does.
- `gascoigne setup -experimental` (and the setup page, in a folded section) turn the experimental features on or off, each with an example, how it works and its impact.
- `gascoigne setup` opens the page in the browser (`-t` asks in the terminal).
- Installs from before this version are found from the agents' config files, so the first session
  after upgrading offers the hook update.

- Documentation diagrams are Mermaid (README, architecture), with new flows for how a command is
  optimized and for upgrading.

### Fixed

- The usage guide no longer tells codex how to write commands for a rewrite hook it lacks, nor
  names MCP tools pi does not have.

## [0.2.1] - 2026-10-12

First release. gascoigne shrinks command output, tool results and web pages before they reach
a coding agent, and keeps every original one command away. Verbose output (installs, test runs,
logs, cloud and Kubernetes JSON/YAML) shrinks 70–99%; across the 268 eval cases the median output
shrinks 69%; in the author's own coding sessions tool output was 4–27% smaller (median 10%).

```sh
go install github.com/gascoigne-ai/gascoigne-cli/cmd/gascoigne@latest && gascoigne wrap claude
```

### In short

- **Shorter tool output:** 16 engines across 110+ tools and formats; verbose output shrinks 70-99%, a typical coding session's tool output 4-27%.
- **Works with your agent:** `wrap` and `install` for Claude Code, Codex, Gemini CLI, opencode, pi and Cursor; `uninstall` removes everything.
- **MCP and API proxies** shorten results from MCP servers and tool results inside LLM requests.
- **Nothing is lost:** every original is kept locally and read back with `gascoigne raw`.
- **See the effect** with a live dashboard, `gain` with cost, and exact token counts with `tokens`.

### Optimizing output

- 16 family engines covering 110+ tools and formats: test, build, pkg, task, vcs, search, fs,
  table, data, log, doc, web, db, k8s, infra and cloud. Output is recognized by family and
  tool and shaped by the engine that knows what it is for: failures, diffs, errors and
  summaries kept, progress, passes and boilerplate folded.
- Stack traces fold recursion, and goroutines parked on one stack are shown once.
- A rerun of the same command in a session points at output the agent already saw instead of
  repeating it.
- Source files only lose whole lines (comments, outlined bodies); kept lines are byte-exact and
  every gap names the lines it stands for. `gascoigne show <file>[:symbol]` prints one
  definition or the file's outline.
- Deterministic: same input, same output, so prompt caches stay warm.

### Agents

- `gascoigne wrap claude|codex|gemini|opencode|pi` starts an agent with gascoigne for one run,
  touching no config files.
- `gascoigne install [-g]` keeps it on for a project or every project (also Cursor);
  `unwrap` removes it from agent configs, `uninstall` removes gascoigne entirely (configs,
  server, ledger, caches and the binary, after asking).
- Agents get a short usage guide and are asked to keep their own text brief (about 16% fewer
  output tokens in A/B runs: 4 tasks × 3 runs on Claude Sonnet).

### MCP and the LLM API proxy

- `gascoigne mcp`: `fetch` for web pages and HTTP APIs, `compress`, `show`, `retrieve`, `stats`.
- `gascoigne mcp-proxy -- <server>` shortens another MCP server's results; `max_tools` hides a
  server with many tools behind `search_tools` and `call_tool` (off by default).
- The LLM API proxy shortens tool results inside requests to the model, trims tool schemas,
  folds long code blocks in invoked skills and can downscale large screenshots (opt-in).
  Works with API keys and gateway tokens, including Claude on Amazon Bedrock; a claude.ai or
  ChatGPT login talks to the provider directly.

### Originals, savings and cost

- Every original goes to a local sqlite ledger first; shortened output carries an 8-digit raw
  id. `gascoigne raw <id>` reads it back whole or in part (`-lines`, `-grep`, `-query`).
- Sources whose originals agents keep fetching are passed through next time.
- `gascoigne gain` reports savings by day, agent, model, session, command or optimizer, with
  a cost section and a note on which numbers are estimates.
- `gascoigne dashboard` opens a live view: savings over time, every run, and any run's
  original side by side with what the agent read. Its moon shows your savings rate.
- `gascoigne tokens` counts and prices text per tokenizer family, with exact o200k counts
  locally and Claude counts through `count_tokens` with an API key.
- Nothing leaves your machine: the ledger and dashboard are local.

### Configuration

- Works without a config file. Overrides go to `~/.config/gascoigne/config.yaml`, per run with
  `-set`, `-only`, `-disable` or `GASCOIGNE_*` variables; `gascoigne config` prints the result
  and `config -upgrade` moves settings onto a newer template.
- Experimental commands are enabled with `experimental: [...]` in the config or
  `GASCOIGNE_EXPERIMENTAL`: `prompts` audits and prices the instruction text sent with every
  request.

### Distribution

- One static, pure-Go binary for Linux, macOS and Windows (amd64, arm64), with checksums.
- Licensed under the Apache License 2.0.

[0.2.1]: https://github.com/gascoigne-ai/gascoigne-cli/releases/tag/v0.2.1
