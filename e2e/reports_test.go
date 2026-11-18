package e2e

// Reports and settings pages with nothing recorded yet: they say so instead of failing.
// Experimental commands answer exit 2 until their feature is switched on.
var _ = group("reports",
	Scenario{Name: "upgrade -n lists the open choices without changing anything", Agents: []string{"claude"}, Steps: []Step{
		{Run: "gascoigne upgrade -n", Stdout: []Check{Has("choices: waiting for you")}, Gone: []string{"${home}/.claude"}},
	}},
	Scenario{Name: "pricing show says where dollar figures come from", Steps: []Step{
		{Run: "gascoigne pricing show", Stdout: []Check{Has("billing: auto")}},
	}},
	Scenario{Name: "probe reports an empty ledger", Steps: []Step{
		{Run: "gascoigne probe", Stdout: []Check{Has("0 optimized originals checked")}},
	}},
	Scenario{Name: "context -text says when there are no transcripts", Steps: []Step{
		{Run: "gascoigne context -text -open=false", Stdout: []Check{Has("No Claude Code transcripts")}},
	}},
	Scenario{Name: "mcp-review -text says when no MCP tool data exists", Steps: []Step{
		{Run: "gascoigne mcp-review -text -open=false", Stdout: []Check{Has("No MCP tool data yet")}},
	}},
	Scenario{Name: "replay without recorded sessions is an error", Steps: []Step{
		{Run: "gascoigne replay", Exit: 1, Stderr: []Check{Has("no such file or directory")}},
	}},
	Scenario{Name: "mcp-login needs a URL", Steps: []Step{
		{Run: "gascoigne mcp-login", Exit: 1, Stderr: []Check{Has("usage: gascoigne mcp-login [-logout] <url>")}},
	}},
	Scenario{Name: "experimental commands exit 2 until their feature is on", Steps: []Step{
		{Run: "gascoigne prompts", Exit: 2, Stderr: []Check{Has(`"prompts" is experimental`)}},
		{Run: "gascoigne learn", Exit: 2, Stderr: []Check{Has(`"learn" is experimental`)}},
		{Run: "gascoigne prose status", Exit: 2, Stderr: []Check{Has(`"prose" is experimental`)}},
	}},
	Scenario{Name: "GASCOIGNE_EXPERIMENTAL switches an experimental command on", Env: []string{"GASCOIGNE_EXPERIMENTAL=prompts"},
		Steps: []Step{
			{Run: "gascoigne prompts", Stderr: []Check{Lacks("is experimental")}},
		}},
	Scenario{Name: "setup -y answers the open questions without a browser", Agents: []string{"claude"}, Steps: []Step{
		{Run: "gascoigne setup -y", Stdout: []Check{Has("claude: set up")}},
	}},
)
