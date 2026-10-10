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

## For good: `wrap -print` and `unwrap`

```sh
gascoigne wrap -print claude    # also codex, pi, cursor
```

prints the config to add to the agent's own files. To remove it again:

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
| Cursor | — (`wrap -print cursor` only) | A `preToolUse` hook returns `updated_input` with `permission: allow`; shell approval stays with `beforeShellExecution`. |

### Claude Code: built-in search tools

Hooks cannot shrink what Claude Code's built-in tools return. `wrap claude` therefore denies
**Grep** and **Glob** for that run, so Claude searches with `rg` and `find` through Bash, where
the hook wraps them. **Read** stays: Edit refuses to change a file Read has not loaded. The
deny rule is not part of `wrap -print`, because `unwrap` could not tell it apart from a rule
you added yourself; add it by hand if you want it for good.

WebFetch is left alone: it returns a short summary of a page, which is usually cheaper than
the page itself. Use the MCP `fetch` tool when you need the page's own text.

## Which commands get rewritten

The hook only rewrites a command when it can do so safely and gascoigne knows the tool:

- **Attribution.** Under `wrap`, `GASCOIGNE_AGENT` and `GASCOIGNE_SESSION` tag every run. Hooks
  installed for good (`wrap -print`) have no such environment, so the rewritten command carries
  them: `gascoigne exec -agent cursor -session <hook session> -- …`. Runs from a terminal or a
  script have no agent; the dashboard shows them as *no agent*.
- **Known tool.** The last command must be detected by `detect.FromCmd` (see
  [Families](families.md)). `echo hi` stays as it is.
- **Lists.** In `a && b` and `a; b` only the last command is wrapped:
  `cd api && go test ./...` → `cd api && gascoigne exec -- go test ./...`.
- **Environment.** Leading assignments stay outside: `CGO_ENABLED=0 gascoigne exec -- go build`.
- **`2>&1`** is kept; exec merges stderr anyway.
- **`| head` / `| tail`.** Trailing head/tail stages are kept and cut the optimized text. Before
  `head`, exec prints its hint first (`-attr hint=top`) so the cut keeps the retrieve id.
- **Left alone:** other pipes (`| grep` would search optimized text), redirects, `$(…)`,
  backticks, `||`, background runs (`&`, `run_in_background`), multi-line scripts, commands
  that run gascoigne itself or set `GASCOIGNE_RAW`, and commands that run until stopped:
  `-f`/`--follow` on logs, `--watch`, `watch`, `dev`, `serve`, `start`, `up`.

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
