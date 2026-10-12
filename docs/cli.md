# CLI reference

```
gascoigne <command> [flags] [args]
```

`gascoigne help` lists the commands by group; `gascoigne help <command>` (or `<command> -h`)
shows its usage, flags, examples and the flags every optimizing command shares. Help is colored
on a terminal unless `NO_COLOR` is set, and a mistyped command gets a suggestion.

## Commands

| Command | What it does |
|---|---|
| `exec [flags] -- <cmd> [args]` | Run a command, print its optimized output (stdout and stderr merged), exit with its exit code. |
| `run [flags]` | Run a pipeline. Default `cli`: stdin in, optimized text out. |
| `raw <id> [-lines a-b \| -grep re [-C n] \| -query words]` | Print an original byte-exact, which marks its source as badly optimized (see [Ledger](ledger.md)); or only part of it, numbered like `grep -n`, which does not. Flags may follow the id. |
| `show <file>[:symbol]` | Print one function, method (`Type.Method`) or type with its doc comment and line numbers, or without a symbol every definition with its line range. Go, Python, C-like languages. |
| `gain [flags]` | Savings report: overview, or one grouping with `-by`. |
| `dashboard` | Start the background server if it is not running and print the dashboard address. |
| `server run\|start\|stop\|status` | The background server hosting the dashboard: one per user, started automatically. |
| `tokens [-model list] [-exact=false] [file]` | Count text with each tokenizer family and price one read and its cost in an agent's context (see [cost](cost.md)); also exact counts: o200k locally, Claude via `count_tokens` when a key is configured. |
| `flush [-older 7d]` | Delete stored originals (the cache behind `raw <id>`) and vacuum the ledger. |
| `context [-since 14d] [-text] [-json]` | Open the context page: how big conversations get, where the money goes, and what compacting earlier would have saved (see [below](#context)). |
| `mcp-review [-text] [-json]` | Open the MCP review page: what each server's tool definitions add to the tool list, calls this week and last, and a verdict (see [below](#mcp-review)). |
| `reset [flags]` | Clear savings stats; `-all` also clears stored originals and failure history. |
| `wrap <agent> [agent args]` | Start `claude`, `codex`, `gemini`, `opencode` or `pi` with gascoigne registered for that run. |
| `install [-g] [agent...]` | Register gascoigne for good in this project's agent configs, or with `-g` in the user's (default: agents found on `PATH`). |
| `wrap -print <agent>` | Print the config that registers gascoigne for good (also `cursor`). |
| `unwrap [agent...]` | Remove gascoigne from agents' config files (default: every agent). |
| `uninstall [-y]` | Remove gascoigne entirely: `unwrap` for every agent, stop the server, delete its config, data (ledger), cache and state dirs and the binary. Asks first. |
| `hook <agent>` | Agent hook handler. Installed by `wrap`; reads the hook event on stdin. |
| `mcp` | MCP server on stdio: `fetch`, `compress`, `show`, `retrieve`, `stats`. |
| `mcp-proxy -- <server cmd>` | MCP proxy: mirrors another server's tools and optimizes their text results. |
| `fail <key\|raw_id>` | Mark a source as badly optimized by hand. `-reason` sets the note. |
| `forget <key>` | Clear a source's failure history so it is optimized again. |
| `eval [flags]` | Measure savings on the fixtures in `testdata/eval` (see [Development](development.md)). |
| `prompts [-agent claude\|codex] [dir]` | Experimental (`experimental: [prompts]`). List the instruction text an agent sends with every request and price it; `-terse 0.1 <files>` drops articles and fillers from instruction files in place (`.bak` kept). |
| `upgrade [-n]` | After installing a new version: write stale or lost hooks, rules and commands of your installs again, move `config.yaml` onto the new template (settings kept, `.bak` left), open the page for choices still waiting. `-n` lists without changing. |
| `doctor` | Report the binary, server, saved answers and each install (hook present, registration current), in plain language. Changes nothing; `upgrade` repairs. |
| `setup [-y] [-all] [-t] [-experimental]` | Answer the open questions (permissions, hook updates, new agents) in a browser page, or in the terminal where no browser opens or with `-t`. `-experimental` asks only about the experimental features (`continuity`, `learn`, `prompts`); `-all` asks answered questions again. |
| `learn [-since 30d] [-apply\|-remove] [-target FILE]` | Experimental (`experimental: [learn]`). Propose instruction lines from habits seen in at least two sessions: commands run again for output that was already shortened, and commands whose large output nothing can shorten. Prints them; `-apply` writes a marked block to `CLAUDE.local.md` (or `-target`), `-remove` takes it out. |
| `components` | List the built-in components and their families. |
| `config [flags]` | Print the effective config after `-set`, `-only`, `-disable` and environment. |

## Flags shared by every command that builds the pipeline

| Flag | Environment | Meaning |
|---|---|---|
| `-config path` | `GASCOIGNE_CONFIG` | Config file. Default: `$XDG_CONFIG_HOME/gascoigne/config.yaml`, else the built-in one. |
| `-set path=yaml` | `GASCOIGNE_SET` (`;`-separated) | Assign any config value, repeatable: `-set optimizers.grep.max_per_file=50`. |
| `-only a,b` | `GASCOIGNE_ONLY` | Keep only these optimizers in every chain. |
| `-disable a,b` | `GASCOIGNE_DISABLE` | Remove these optimizers from every chain. |
| `-attr k=v` | | Set a payload attribute, repeatable: `-attr cmd='git status'`, `-attr query=timeout`. |
| `-session id` | `GASCOIGNE_SESSION` | Tag events with an agent session, for `gain -session`. |
| `-agent name` | `GASCOIGNE_AGENT` | Tag events with the agent name. MCP defaults to the client's name. |
| `-model name` | `GASCOIGNE_MODEL` | Tag events with the model the output is for. |
| `-v` | | Debug logs to stderr. |

Flags win over environment. `GASCOIGNE_SET` is applied before `-set` flags.

`run` and `exec` also take:

| Flag | Meaning |
|---|---|
| `-raw` | Skip optimization; output verbatim (same as `GASCOIGNE_RAW=1`). |
| `-pipeline name` | Run another pipeline from the config (`exec` uses `exec`, `run` uses `cli`). |
| `-watch` | `exec` only: the command rebuilds until stopped (`tsc -w`); print each cycle, optimized, when it ends. |

## `gain`

```sh
gascoigne gain                          # overview
gascoigne gain -by command              # one grouping
gascoigne gain -since 7d -by day        # trend
gascoigne gain -session claude-2026…    # one agent session
gascoigne gain -failures                # sources that bypass optimization
gascoigne gain -json                    # machine-readable
```

| Flag | Meaning |
|---|---|
| `-by g` | `day`, `week`, `month`, `agent`, `model`, `session`, `command`, `optimizer`, `family`, `dialect`, `outcome`, `reason`, `route` |
| `-since d` | Only the last period: `24h`, `7d`, `4w`, `6mo` |
| `-session`, `-agent`, `-model` | Filters, combine freely |
| `-all` | Ignore session/agent/model coming from `GASCOIGNE_*` (inside a wrapped agent, plain `gain` shows only the current session) |
| `-failures` | List failing sources |
| `-json` | JSON output |
| `-color auto\|always\|never` | `auto` honors `NO_COLOR` and whether stdout is a terminal |

## `server` and `dashboard`

```sh
gascoigne dashboard            # start the server if needed, print http://127.0.0.1:7878
gascoigne server status        # pid, uptime, ledger, build
gascoigne server stop
gascoigne server start         # start in the background and wait until it answers
gascoigne server run           # run in the foreground (what the background process runs)
```

All take `-addr host:port` (default `$GASCOIGNE_SERVER_ADDR` or `127.0.0.1:7878`). `exec`,
`hook`, `mcp`, `mcp-proxy` and `wrap` start the server when it is not answering, so the
dashboard is up whenever gascoigne is used. See [Ledger, retrieval and gain](ledger.md#dashboard).

## `context`

```sh
gascoigne context             # opens the page in the dashboard
gascoigne context -since 24h -text     # also 7d, 4w, ...
gascoigne context -json
```

Every request re-reads the whole conversation, so a session costs mostly requests times context,
not the size of any one tool result. `context` reads Claude Code's own transcripts
(`~/.claude/projects`, which hold the usage of every request with a session id; the LLM proxy sees
no sessions) and shows, at API list prices: the cost by the size of the conversation each request
re-read, how much of it was written to the cache again instead of read (after a pause longer than
the cache lives, after a compaction, or when the cached start of the conversation changed), the text added to conversations besides your prompts (instruction files, the skill listing and what hooks and
plugins print, named by the command they ran, with its cost), your largest
sessions with the context over time, and what compacting at 150k, 250k or
400k tokens would have saved. On one real machine 42% of a month's cost came from requests above 400k
tokens of context.

The what-if uses one simple model for every threshold (cache reads for what was cached, writes for what
is new, one request over the whole context plus a 12k-token summary per compaction), so the numbers
compare with each other; its baseline came within 5% of the actual cost there. Dollars are list prices
with 5-minute cache writes: a subscription does not bill per token, and 1-hour writes cost more.
**Compacting saves tokens, not detail**: the saving is an upper bound on what is worth giving up, not
advice to compact everywhere. Only main conversations count, subagents have conversations of their own. A session that spans the
period counts only the requests inside it (the page offers 24 hours to 90 days).
Nothing leaves the machine and nothing is changed. Without a display, under `CI` or with
`GASCOIGNE_SERVER=off` the table is printed; `-open=false` only prints the page's address. Claude Code
only: other agents keep no transcripts of this shape.

## `mcp-review`

```sh
gascoigne mcp-review          # opens the page in the dashboard
gascoigne mcp-review -text    # the same table in the terminal
gascoigne mcp-review -json
```

Most agents send the definitions of all their tools with every request, so an MCP server that is never
called is paid for all week. An agent with its own tool search (Claude Code defers many tools behind
one) may send only names, so treat the numbers as an upper bound for it. The page lists each server with its definition tokens,
the tokens of tools not called in 14 days, calls in the last 7 days and the 7 before, and a verdict:
`used`, `rarely used` (at most two calls a week for a server whose definitions are 1,500 tokens or more),
`unused` (no call in 14 days), `learning` (seen for less than a week, so no verdict) or `not seen
lately` (its tools were last listed over a week ago: probably removed, so it is neither called unused nor
counted for the weekly note).
Click a server for its tools and for the one action the page offers: loading the tools it never uses on
request instead of listing them (see [mcp](mcp.md#proxy-gascoigne-mcp-proxy)). Without a display, under `CI`, or with `GASCOIGNE_SERVER=off` the
table is printed instead; `-open=false` only prints the page's address.

The data is what `mcp-proxy` records when the agent lists tools and calls them (names, sizes and
counts only), kept two weeks (`ledger.stats_ttl`). Servers that stay direct have no sizes and show
only once the agent has called them. For a server started through `mcp-proxy` the page also finds the
config entry that starts it (Claude's user, local and project files, Cursor's `mcp.json`; matched by the hash
of its command) and shows the command that switches it off; gascoigne does not run it. Looking changes nothing; the page's one action, loading unused
tools on request, is saved only when you press its button.

## `reset`

```sh
gascoigne reset -session claude-20261010-111234   # one session's runs
gascoigne reset -agent codex -since 24h
gascoigne reset                                   # every recorded run
gascoigne reset -all                              # also stored originals and failure history
```

Deletes recorded runs (what `gain` and the dashboard show). Filters come from flags only, never
from `GASCOIGNE_*` variables, so it deletes exactly what it says. `-all` cannot be combined with
filters: originals and failure history carry no session or agent.

## `flush`

```sh
gascoigne flush               # every stored original
gascoigne flush -older 7d     # only older ones
```

Deletes originals and prints how much space went. Recorded runs and savings stay.

## `eval`

| Flag | Meaning |
|---|---|
| `-cases file` | Cases file; fixture paths are relative to it. Default `testdata/eval/cases.yaml`. |
| `-show substr` | Print the optimized output of cases whose name contains this. |
| `-update` | Rewrite golden outputs after you reviewed the change. |
| `-pipeline name` | Pipeline to evaluate (default `cli`). |

## Environment variables

| Variable | Effect |
|---|---|
| `GASCOIGNE_RAW=1` | Bypass optimization for every payload. |
| `GASCOIGNE_CONFIG` | Config file path. |
| `GASCOIGNE_LEDGER` | Ledger database path (overrides the config). |
| `GASCOIGNE_SET`, `GASCOIGNE_ONLY`, `GASCOIGNE_DISABLE` | Per-run config overrides, as the flags above. |
| `GASCOIGNE_SESSION`, `GASCOIGNE_AGENT`, `GASCOIGNE_MODEL` | Identity tags for events. `wrap` sets the first two. |
| `GASCOIGNE_CAT_MODE` | Source file mode: `auto`, `full`, `clean`, `outline` (see [Source files](source-files.md)). |
| `GASCOIGNE_SERVER=off` | Never start or use the background server: every command builds its own pipeline. |
| `GASCOIGNE_SERVER_IDLE` | How long the background server waits without a request before it exits (default `30m`, `0` never). |
| `GASCOIGNE_SERVER_ADDR` | Server address (default `127.0.0.1:7878`). |
| `GASCOIGNE_MCP_LAZY` | `off`: `mcp-proxy` ignores the tools chosen to load on request (see [mcp](mcp.md#proxy-gascoigne-mcp-proxy)); the saved choice stays. |
| `GASCOIGNE_PROXY` | `off`: agents never use the LLM proxy. `on`: Claude uses it even with a claude.ai login (see [agents](agents.md#the-llm-proxy-and-a-claudeai-login)). Unset: Claude only with an API key or gateway token. |
| `GASCOIGNE_MCP_REVIEW=off` | A session start never opens the weekly MCP review page (see [agents](agents.md#weekly-mcp-review)). |
| `GASCOIGNE_RELEASE_CHECK=off` | Never read the website for a newer release (see [agents](agents.md#release-notice)). |
| `GASCOIGNE_RELEASE_URL` | Another site address to read `latest.json` from (a mirror, a test). |
| `GASCOIGNE_GUIDE` | Usage text for agents: `off`, `short` or `full` (overrides `install -guide`). |
| `GASCOIGNE_AUTOSYNC=off` | Never let agent hooks update installs after an upgrade. |
| `GASCOIGNE_EXPERIMENTAL` | Enable experimental features (`prompts`, `learn`, `continuity`, or `all`). |
| `NO_COLOR` | No colors in `gain`. |

## Exit codes

`exec` exits with the command's exit code. `hook` always exits 0 so it never breaks an
agent's tool call. Other commands exit 1 on error.
