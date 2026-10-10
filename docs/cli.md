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
| `raw <id>` | Print an original byte-exact. Also marks its source as badly optimized (see [Ledger](ledger.md)). |
| `gain [flags]` | Savings report: overview, or one grouping with `-by`. |
| `dashboard` | Start the background server if it is not running and print the dashboard address. |
| `server run\|start\|stop\|status` | The background server hosting the dashboard: one per user, started automatically. |
| `tokens [-model list] [file]` | Count text with each tokenizer family and price one read and its cost in an agent's context (see [cost](cost.md)). |
| `flush [-older 7d]` | Delete stored originals (the cache behind `raw <id>`) and vacuum the ledger. |
| `reset [flags]` | Clear savings stats; `-all` also clears stored originals and failure history. |
| `wrap <agent> [agent args]` | Start `claude`, `codex` or `pi` with gascoigne registered for that run. |
| `install [-g] [agent...]` | Register gascoigne for good in this project's agent configs, or with `-g` in the user's (default: agents found on `PATH`). |
| `wrap -print <agent>` | Print the config that registers gascoigne for good (also `cursor`). |
| `unwrap [agent...]` | Remove gascoigne from agents' config files (default: every agent). |
| `hook <agent>` | Agent hook handler. Installed by `wrap`; reads the hook event on stdin. |
| `mcp` | MCP server on stdio: `fetch`, `compress`, `retrieve`, `stats`. |
| `proxy [-addr host:port]` | LLM API proxy (default `127.0.0.1:7879`): shortens tool results in agents' requests, Claude's Read included, with line numbers kept. `wrap` starts it; see [agents](agents.md#llm-api-proxy). |
| `mcp-proxy -- <server cmd>` | MCP proxy: mirrors another server's tools and optimizes their text results. |
| `fail <key\|raw_id>` | Mark a source as badly optimized by hand. `-reason` sets the note. |
| `forget <key>` | Clear a source's failure history so it is optimized again. |
| `eval [flags]` | Measure savings on the fixtures in `testdata/eval` (see [Development](development.md)). |
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
| `GASCOIGNE_SERVER_ADDR` | Server address (default `127.0.0.1:7878`). |
| `NO_COLOR` | No colors in `gain`. |

## Exit codes

`exec` exits with the command's exit code. `hook` always exits 0 so it never breaks an
agent's tool call. Other commands exit 1 on error.
