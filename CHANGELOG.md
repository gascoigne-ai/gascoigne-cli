# Changelog

All notable changes to gascoigne. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/).

## [0.6.0] - 2026-10-14

A second prose model you can pick in setup, a progress bar for its download, and an API for agent
framework SDKs. The prose model stays an experiment.

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

### Fixed

- The release notice read `latest.json` from the website's old address (404 since the site moved to
  `gascoigne-ai.github.io`), so no update was ever signalled.

## [0.4.0] - 2026-10-13

Prebuilt binaries on every release.

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
