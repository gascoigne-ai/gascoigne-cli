# Agents (`cmd/gascoigne/agents.go`, `hook.go`)

## Commands

| Command | Does |
|---|---|
| `wrap <agent> [args]` | starts the agent with gascoigne registered for that run only (no config files touched), `GASCOIGNE_AGENT`/`GASCOIGNE_SESSION` set |
| `install [-g] [agent...]` | writes the registering config into project files (`-g`: user files). Existing gascoigne entries replaced, JSON merged, backup kept (`install.go`) |
| `wrap -print <agent>` | prints that config |
| `unwrap [agent...]` | (default: all) removes exactly those entries from user and project configs: claude settings + `.mcp.json` + `claude mcp remove`, cursor hooks/mcp JSON, codex `config.toml` tables, pi extension. Other keys keep their order; the original is saved as `<file>.gascoigne-bak`; unparsable files are reported, not rewritten |
| `uninstall [-y]` | (`uninstall.go`) unwrap all + stop the server + delete gascoigne's config/data/cache/state dirs, an outside ledger's files (never its dir) and the binary; asks first |

## Per agent

| Agent | Per run | How shell output is optimized |
|---|---|---|
| claude | `--mcp-config` + `--settings`. Denies Grep/Glob (searches run through Bash) and WebFetch (pages go through gascoigne fetch); Read stays (Edit needs it); WebSearch stays | PreToolUse rewrites the command to `gascoigne exec -- <cmd>`. No permission decision: rules see the wrapped command |
| codex | `-c mcp_servers…` + `-c hooks.PostToolUse…` | PostToolUse runs the output through pipeline `hook` and replaces the result (`decision: block`); codex rewrites a command only by approving it. Trust the hook once with `/hooks` |
| pi | `--extension` (written to the user cache dir) | `tool_call` rewrites the bash command; no MCP |
| gemini | env `GEMINI_CLI_SYSTEM_SETTINGS_PATH` → run settings in the cache dir (real system settings merged in, user/project stdio servers proxied) | BeforeTool `hookSpecificOutput.tool_input` rewrites `run_shell_command` (no decision: approval stays); SessionStart adds the guide (`gemini.go`) |
| opencode | env `OPENCODE_CONFIG_DIR` (plugin in the cache dir) + `OPENCODE_CONFIG_CONTENT` (`mcp`, proxied servers, `instructions` guide unless the user has some) | plugin `tool.execute.before` rewrites `output.args.command` via `gascoigne hook opencode` (`opencode.go`) |
| cursor | — (config files only: `-print`) | preToolUse `updated_input`, `permission: allow` (shell approval stays with `beforeShellExecution`) |

## Usage guide (`guide.go`)

claude: a SessionStart hook (`additionalContext`). codex: `-c developer_instructions` per run, or
an `AGENTS.md` marked block. cursor: a project rule. pi: an `AGENTS.md` block. Unwrap removes them.

`guide(rewrites, mcp)` says only what the agent has: rewrite advice where commands are rewritten
(not codex), the fetch/compress/show/retrieve tools where gascoigne's MCP server is registered
(not pi; it gets the `gascoigne show` / `gascoigne raw` CLI forms). Codex and pi share the project
`AGENTS.md` block: the one installed last stays.

Level: `$GASCOIGNE_GUIDE`, else `guide` in `prefs.json` (state dir, `prefs.go`; set with
`install -guide`), else `full`. `short` keeps the path-id and hint lines; `off` injects nothing
and the next `install` removes the AGENTS.md block and cursor rule. Unknown values mean full.
The file is not `config.yaml`: a key there needs a change in `internal/config`.

## Other MCP servers (`proxy.go`)

Stdio servers start through `gascoigne mcp-proxy -- <cmd>`; remote servers stay direct.
- `wrap` passes same-named copies: claude `--mcp-config` beats configured scopes for the run
  (approved/enabled servers only); codex uses `-c` overrides.
- `install` rewrites `.mcp.json`, `~/.claude.json`, codex tables and cursor `mcp.json`.
- `unwrap` restores the original commands from the kept args.
