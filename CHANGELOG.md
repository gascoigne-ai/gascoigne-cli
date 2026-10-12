# Changelog

All notable changes to gascoigne. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

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
