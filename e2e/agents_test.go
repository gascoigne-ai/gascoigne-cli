package e2e

// Registering gascoigne with coding agents. `claude` here is e2estub in disguise: it records how
// it was started to ${record} and exits with $E2E_EXIT. Nothing touches a real agent config.
var _ = group("agents",
	Scenario{Name: "wrap starts the agent with its arguments, registers gascoigne for that run and keeps its exit code",
		Agents: []string{"claude"}, Env: []string{"E2E_EXIT=3"},
		Steps: []Step{
			{Run: "gascoigne wrap claude --foo bar", Exit: 3, Files: map[string][]Check{"${record}": {
				JSONField("agent", "claude"),
				JSONField("env.GASCOIGNE_AGENT", "claude"),
				Has(`"--mcp-config"`, `"--foo","bar"`, "${bin}/gascoigne hook claude"),
				Like(`"GASCOIGNE_SESSION":"claude-\d{8}-\d{6}"`)}}},
			{Run: "gascoigne wrap claude", Exit: 3, Gone: []string{"${home}/.claude", "${work}/.claude", "${work}/.mcp.json"}},
		}},
	Scenario{Name: "wrap -print shows the config that registers gascoigne for good", Steps: []Step{
		{Run: "gascoigne wrap -print claude", Stdout: []Check{
			Has("merge into ~/.claude/settings.json", `"PreToolUse"`, "${bin}/gascoigne hook claude")}},
		{Run: "gascoigne wrap -print cursor", Stdout: []Check{Has("merge into ~/.cursor/hooks.json", "hook cursor")}},
	}},
	Scenario{Name: "wrap with a missing or unknown agent is an error", Steps: []Step{
		{Run: "gascoigne wrap", Exit: 1, Stderr: []Check{Has("want an agent: claude, codex, cursor, gemini, opencode, pi")}},
		{Run: "gascoigne wrap nosuch", Exit: 1, Stderr: []Check{Has(`unknown agent "nosuch"`)}},
	}},
	Scenario{Name: "install registers hooks and the MCP server in this project only", Agents: []string{"claude"},
		Steps: []Step{
			{Run: "gascoigne install claude", Stdout: []Check{Has("added PreToolUse, PostToolUse and SessionStart hooks")},
				Files: map[string][]Check{
					"${work}/.claude/settings.json": {Has("PreToolUse", "GASCOIGNE_AGENT", "hook claude")},
					"${work}/.mcp.json":             {Has("gascoigne", `"mcp"`)}},
				Gone: []string{"${home}/.claude/settings.json"}},
		}},
	Scenario{Name: "install -g registers in the user's config instead", Agents: []string{"claude"}, Steps: []Step{
		{Run: "gascoigne install -g claude", Files: map[string][]Check{
			"${home}/.claude/settings.json": {Has("PreToolUse", "hook claude")}},
			Gone: []string{"${work}/.claude", "${work}/.mcp.json"}},
	}},
	Scenario{Name: "unwrap removes what install added and keeps a backup", Agents: []string{"claude"}, Steps: []Step{
		{Run: "gascoigne install claude"},
		{Run: "gascoigne unwrap claude", Stdout: []Check{Has("removed PreToolUse hook")},
			Files: map[string][]Check{
				"${work}/.claude/settings.json":               {Lacks("PreToolUse", "GASCOIGNE_AGENT")},
				"${work}/.claude/settings.json.gascoigne-bak": {Has("PreToolUse")}},
			Gone: []string{"${work}/.claude/skills/gascoigne-doctor"}},
	}},
	Scenario{Name: "uninstall -y removes the registration, config, data and the binary", Agents: []string{"claude"},
		Steps: []Step{
			{Run: "gascoigne install -g claude"},
			{Run: "gascoigne uninstall -y", Stdout: []Check{Has("gascoigne: uninstalled")},
				Files: map[string][]Check{"${home}/.claude/settings.json": {Lacks("PreToolUse")}},
				Gone:  []string{"${bin}/gascoigne", "${home}/.config/gascoigne", "${home}/.local/state/gascoigne"}},
		}},
	Scenario{Name: "hook always exits 0, even on input it cannot read", Steps: []Step{
		{Run: "gascoigne hook claude", Stdin: "garbage\n", Stderr: []Check{Has("read event")}},
	}},
	Scenario{Name: "doctor reports the install state and changes nothing", Agents: []string{"claude"}, Steps: []Step{
		{Run: "gascoigne doctor", Stdout: []Check{Has("GASCOIGNE_SERVER=off", "claude: found on this machine")},
			Gone: []string{"${home}/.claude", "${work}/.claude"}},
	}},
)
