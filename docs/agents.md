# Agents

gascoigne helps an agent in two ways:

1. **Shell output.** Commands the agent runs go through `gascoigne exec`, or their output is
   optimized after they ran.
2. **MCP tools.** `fetch` reads web pages and APIs compactly, `compress` shrinks any text,
   `retrieve` returns originals, `stats` reports savings. See [MCP](mcp.md).

## Per run: `wrap`

```sh
gascoigne wrap claude [claude args…]
gascoigne wrap codex  [codex args…]
gascoigne wrap pi     [pi args…]
```

`wrap` starts the agent with gascoigne registered for that run only. No config file is
touched. It first makes sure the background server runs and prints the dashboard address. It also sets `GASCOIGNE_AGENT` (the agent name) and `GASCOIGNE_SESSION`
(`<agent>-<date>-<time>`), so every event of the run is tagged and `gascoigne gain` inside the
agent shows that session.

## For good: `install` and `unwrap`

```sh
gascoigne install               # this project: agents found on PATH
gascoigne install claude codex  # this project: named agents
gascoigne install -g            # every project: the user's config files
gascoigne wrap -print claude    # only print the config, to add it by hand
```

| Agent | `install` (project) | `install -g` (user) |
|---|---|---|
| claude | `.claude/settings.json` hook + env, `.mcp.json` | `~/.claude/settings.json`, `claude mcp add --scope user` (else `~/.claude.json`) |
| codex | `.codex/config.toml` | `~/.codex/config.toml` (`$CODEX_HOME`) |
| cursor | `.cursor/hooks.json`, `.cursor/mcp.json` | `~/.cursor/hooks.json`, `~/.cursor/mcp.json` |
| pi | `.pi/extensions/gascoigne.ts` | `~/.pi/agent/extensions/gascoigne.ts` |
| gemini | `.gemini/settings.json` (hooks, MCP) | `~/.gemini/settings.json` |
| opencode | `opencode.json`, `.opencode/plugins/gascoigne.ts`, `AGENTS.md` | `~/.config/opencode/opencode.json`, `plugins/gascoigne.ts`, `AGENTS.md` |

Entries are merged into what is there; a changed file is saved as `<file>.gascoigne-bak`.
Earlier gascoigne entries are replaced, so running `install` again after moving the binary
refreshes them, and an up-to-date file is left untouched. Configs call plain `gascoigne` when
`PATH` finds this binary (project files can then be committed), else its full path. Claude asks
to approve a project `.mcp.json` server on first use; Codex asks to trust the hook (`/hooks`).
To remove it again:

```sh
gascoigne unwrap                # every agent
gascoigne unwrap claude codex
```

`unwrap` removes exactly gascoigne's entries from user and project configs: Claude settings,
`.mcp.json` and `claude mcp remove`, Cursor hooks and MCP JSON, Codex `config.toml` tables and
the pi extension. Other keys keep their order. The original file is saved as
`<file>.gascoigne-bak`. Files it cannot parse are reported, not rewritten.

## Per agent

| Agent | `wrap` passes | How shell output is optimized |
|---|---|---|
| Claude Code | `--mcp-config`, `--settings` | A `PreToolUse` hook on Bash rewrites the command to `gascoigne exec -- <cmd>`. It makes no permission decision, so your permission rules see the wrapped command. |
| Codex | `-c mcp_servers…`, `-c hooks.PostToolUse…` | A `PostToolUse` hook optimizes the output after the run and replaces the result. Codex can only rewrite a command by approving it, so it is optimized afterwards instead. Trust the hook once with `/hooks`. |
| pi | `--extension` (written to your cache dir) | The extension rewrites the bash command through `gascoigne hook pi`. pi has no MCP. |
| gemini | `GEMINI_CLI_SYSTEM_SETTINGS_PATH` to a settings file in your cache dir (your real system settings are merged in) | A BeforeTool hook rewrites `run_shell_command` through `gascoigne hook gemini`; approval stays yours. |
| opencode | `OPENCODE_CONFIG_DIR` (plugin) and `OPENCODE_CONFIG_CONTENT` (MCP servers) | The plugin's `tool.execute.before` rewrites the bash command. Not available when you set `OPENCODE_CONFIG_DIR` yourself: use `install`. |
| Cursor | — (`wrap -print cursor` only) | A `preToolUse` hook returns `updated_input` with `permission: allow`; shell approval stays with `beforeShellExecution`. |

### Results the rewrite cannot reach

Rewriting a command before it runs covers most shell output. What it leaves is optimized after the
tool ran, by replacing the result the model sees:

| Agent | Hook | Covers |
|---|---|---|
| claude | PostToolUse `updatedToolOutput` (same shape as the tool's output) | Bash commands left as they were (pipes, redirects, loops, background runs), Grep in content mode, every MCP server's results (remote, plugin and claude.ai ones included) |
| codex | PostToolUse `decision: block` + `reason` | MCP servers' results (shell output already) |
| gemini | AfterTool `decision: deny` + `reason` | shell commands left as they were, MCP servers' results |

Output gascoigne already optimized (exec, its MCP server, the proxy) is recognized by its hint and
left alone, as are gascoigne's own MCP tools. **Read** is never replaced: Claude numbers the lines
it shows, and dropped lines would shift those numbers. **WebSearch** is left too: its results
(a median of 608 tokens in measured sessions) are rendered by Claude Code from a fixed structure.

### LLM API proxy

What hooks cannot reach (Claude's Read, a quarter of tool-result tokens in measured sessions,
plus WebSearch, subagent results and plans) is shortened between the agent and its API by
the LLM proxy the background server serves under `/v1/` (same process and port as the
dashboard):

| Agent | Per run (`wrap`) | For good |
|---|---|---|
| claude | `ANTHROPIC_BASE_URL=http://127.0.0.1:7878`, unless you set one (your gateway stays) | `install -proxy claude`; the SessionStart hook starts the server |
| codex | with `OPENAI_API_KEY`: a `gascoigne` model provider (Responses API); a ChatGPT login stays direct | — |
| opencode | `provider.anthropic/openai.options.baseURL` | — |

- Only the base URL changes: a claude.ai login stays the credential, and every header goes
  through as sent.
- Read results keep their real line numbers: lines are dropped like `cat` output, never
  renumbered, and each gap is a marker naming the lines to read back.
- Each tool result is shortened once and sent byte-for-byte the same on every later request,
  so the provider's prompt cache keeps hitting. Anything the proxy cannot parse is forwarded
  unchanged; `X-Gascoigne-Raw: 1` on a request skips it.
- `wrap` starts the server when it is not answering and leaves the agent on its API directly
  if it cannot. `GASCOIGNE_PROXY=off` (or `GASCOIGNE_SERVER=off`) turns it off. When the
  server replaces itself with a new build, it first waits until no proxied request is in
  flight. With `install -proxy`, a stopped server stops the agent until the next session
  starts it.

### Claude Code: built-in search and fetch tools

`wrap claude` and `install claude` deny **Grep**, **Glob** and **WebFetch**: Claude then searches
with `rg` and `find` through Bash, where the rewrite optimizes them fully, and reads pages with
gascoigne's `fetch`, whole and compressed, where WebFetch hands back a small model's retelling of
the page. **Read** stays: Edit refuses to change a file Read has not loaded. `install` records the
rules it added (`$XDG_STATE_HOME/gascoigne/denies.json`), so `unwrap` removes those and keeps
deny rules you wrote yourself.

## Usage guide for the agent

Agents are told how to get the most out of gascoigne: read and search with shell commands,
keep shell calls plain (`&&`/`;` lists, only `| head`/`| tail`), read the web with `fetch`, and
use `gascoigne raw <id>` / `retrieve` instead of re-running a shortened command.

| Agent | `wrap` | `install` | `install -g` |
|---|---|---|---|
| claude | `SessionStart` hook (`additionalContext`; again after compaction and `/clear`) | same hook in `.claude/settings.json` | in `~/.claude/settings.json` |
| codex | `-c developer_instructions` (skipped when you set your own) | marked block in `AGENTS.md` | in `~/.codex/AGENTS.md` |
| cursor | — | rule `.cursor/rules/gascoigne.mdc` (`alwaysApply`) | none: Cursor keeps user rules in its settings |
| pi | — | marked block in `AGENTS.md` | in `~/.pi/agent/AGENTS.md` |
| gemini | SessionStart hook | same hook in `settings.json` | same |
| opencode | `instructions` file (skipped when you list your own) | marked block in `AGENTS.md` | in `~/.config/opencode/AGENTS.md` |

The `AGENTS.md` block sits between `<!-- gascoigne:start -->` and `<!-- gascoigne:end -->`;
`install` replaces it, `unwrap` removes it (and deletes the file when nothing else is in it).

## Other MCP servers

Every other stdio MCP server the agent has is started through `gascoigne mcp-proxy`, so its
tool results are optimized too. `{"command": "npx", "args": ["-y", "srv"]}` becomes
`{"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "srv"]}`; env and other keys
stay, and the original command stays in `args`, so `unwrap` restores it exactly.

| Agent | `wrap` (this run) | `install` |
|---|---|---|
| claude | same-named copies in `--mcp-config`, which replace the configured ones for the run (plugins and claude.ai connectors keep loading). User, local and approved `.mcp.json` servers; disabled or unapproved ones are not started | `.mcp.json` and this project's local servers in `~/.claude.json`; `-g` the user's servers in `~/.claude.json` |
| codex | `-c mcp_servers.<name>.command/args` overrides | `config.toml` tables rewritten |
| cursor | — | `mcp.json` rewritten |
| gemini | proxied copies in the run settings (system scope wins) | `settings.json` `mcpServers` rewritten |
| opencode | proxied copies in `OPENCODE_CONFIG_CONTENT` | `opencode.json` `mcp` commands rewritten |

Remote servers (`http`, `sse`, `ws`, `url`) are listed as *left direct*: their auth (OAuth,
headers) belongs to the agent. Plugin servers and claude.ai connectors cannot be proxied.
Claude Code keeps `~/.claude.json` in memory and writes it back, so run `install` for claude
with no Claude session open.

## Which commands get rewritten

The hook rewrites every command it can rewrite safely:

- **Attribution.** Under `wrap`, `GASCOIGNE_AGENT` and `GASCOIGNE_SESSION` tag every run. Hooks
  installed for good (`wrap -print`) have no such environment, so the rewritten command carries
  them: `gascoigne exec -agent cursor -session <hook session> -- …`. Runs from a terminal or a
  script have no agent; the dashboard shows them as *no agent*.
- **Any program.** Unknown tools and scripts are wrapped too (`./report.py --json`): their output
  is detected by content (JSON, logs, tables, see [Families](families.md)). Left out: shell
  builtins (`cd`, `export`, `source`: they must change the agent's shell), commands that print
  little (`echo`, `mkdir`, `rm`, `mv`, `cp`, `chmod`, …: they would only add bypassed runs to
  the stats), editors and pagers, and REPLs started without arguments (`python3`, `psql`).
- **Lists.** In `a && b` and `a; b` every command is wrapped, each with its own hint:
  `cd api && go test ./...` → `cd api && gascoigne exec -- go test ./...`;
  `git status; git log -5` → `gascoigne exec -- git status; gascoigne exec -- git log -5`.
  Lines with loops, conditionals or `{ }` groups are left whole.
- **Environment.** Leading assignments stay outside: `CGO_ENABLED=0 gascoigne exec -- go build`.
- **`2>&1`** is kept; exec merges stderr anyway.
- **`| head` / `| tail`.** Trailing head/tail stages are kept and cut the optimized text. Before
  `head`, exec prints its hint first (`-attr hint=top`) so the cut keeps the retrieve id.
- **Left alone:** other pipes (`| grep` would search optimized text), redirects, `$(…)`,
  backticks, `||`, background runs (`&`, `run_in_background`), multi-line scripts, commands
  that run gascoigne itself or set `GASCOIGNE_RAW`, and commands that run until stopped:
  `-f`/`--follow` on logs, `--watch`, `watch`, `dev`, `serve`, `start`, `up`, plain `vite`.
- **Build tools in watch mode** (`tsc -w`, `webpack --watch`, `webpack serve`, `vite build
  --watch`, rollup, esbuild) run as `gascoigne exec -watch -- …`, also in the background: each
  rebuild is printed, optimized, as soon as its result line (`Found 2 errors. Watching for file
  changes.`, `webpack … compiled …`) appears.

## Reading results as an agent

Optimized output ends with (or, before `head`, starts with) a hint:

```
[gascoigne: optimized 4120→610 tokens · full: gascoigne raw <id>]
```

- Need the full text? `gascoigne raw <id>`, or MCP `retrieve` with the id.
- Need one command verbatim? `gascoigne exec -raw -- <cmd>` or `GASCOIGNE_RAW=1`.
- Source files show gaps as comments naming line ranges; read a range with `sed -n 12,30p f`,
  which is small and passes through untouched. See [Source files](source-files.md).

Fetching an original teaches gascoigne that the source needed more, so the next run of the
same command, file or URL passes through untouched.

## Tuning per agent

No config file is needed: set flags in the wrapped command or environment in the agent.

```sh
GASCOIGNE_SET='optimizers.grep.max_per_file=30' gascoigne wrap claude
GASCOIGNE_DISABLE=cat gascoigne wrap codex     # leave source files alone
```

See [Configuration](configuration.md).

## Watching it work

```sh
gascoigne dashboard
```

shows every run as it happens, with the agent, command, content type, savings and, for runs
that were not optimized, why. See [Ledger, retrieval and gain](ledger.md#dashboard).
