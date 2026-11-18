package e2e

const mcpLog = "@testdata/eval/go/test-fail.txt"

// `mcp` is gascoigne's own tool server, `mcp-proxy` wraps another server. Both are driven by a
// real MCP client over stdio. mcpmock is the stand-in upstream server.
var _ = group("mcp",
	Scenario{Name: "mcp serves fetch, compress, show, retrieve and stats", Steps: []Step{
		{Run: "gascoigne mcp", MCP: []Call{{Tool: "stats"}},
			Stdout: []Check{Is("compress\nfetch\nretrieve\nshow\nstats\n")}},
	}},
	Scenario{Name: "mcp compress shortens text and retrieve returns the original", Files: map[string]string{
		"log.txt": mcpLog},
		Steps: []Step{
			{Run: "gascoigne mcp", MCP: []Call{
				{Tool: "compress", Args: map[string]any{"text": mcpLog},
					Save: map[string]string{"id": `\[raw:([0-9a-f]+)\]`}, Text: []Check{ShorterThan("log.txt")}},
				{Tool: "retrieve", Args: map[string]any{"id": "${id}"}, Text: []Check{SameAsFile("log.txt")}},
			}},
		}},
	Scenario{Name: "mcp show prints one symbol of a source file", Files: map[string]string{
		"calc.go": "package calc\n\n// Total sums the prices.\nfunc Total(prices []int) int {\n\treturn 0\n}\n"},
		Steps: []Step{
			{Run: "gascoigne mcp", MCP: []Call{
				{Tool: "show", Args: map[string]any{"file": "${work}/calc.go", "symbol": "Total"}, Text: []Check{Has("func Total")}},
			}},
		}},
	Scenario{Name: "mcp-proxy mirrors the upstream tools", Steps: []Step{
		{Run: "gascoigne mcp-proxy -- mcpmock serve", MCP: []Call{{Tool: "get_status"}},
			Stdout: []Check{Has("list_issues", "search_code", "read_file", "get_logs", "get_status")}},
	}},
	Scenario{Name: "mcp-proxy shortens a big result and leaves a small one alone", Steps: []Step{
		{Run: "gascoigne mcp-proxy -- mcpmock serve", MCP: []Call{
			{Tool: "get_status", Text: []Check{Has(`"status":"ok"`), Lacks("[raw:")}},
			{Tool: "get_logs", Text: []Check{Has("[raw:")}},
		}},
	}},
)
