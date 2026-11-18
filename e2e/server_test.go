package e2e

// The background server hosts the dashboard and is shared by commands. Each scenario gets a
// server of its own on a free port.
var _ = group("server",
	Scenario{Name: "server status says when no server runs", Slow: true, Server: true, Steps: []Step{
		{Run: "gascoigne server status", Stdout: []Check{Like(`^gascoigne server: not running on 127\.0\.0\.1:\d+\n$`)}},
	}},
	Scenario{Name: "server start, status and stop run the server's life", Slow: true, Server: true, Steps: []Step{
		{Run: "gascoigne server start", Stdout: []Check{Has("gascoigne dashboard: http://127.0.0.1:")}},
		{Run: "gascoigne server status", Stdout: []Check{Has("running on http://127.0.0.1:", "exits after 1m0s idle", "ledger ${root}/ledger.db")}},
		{Run: "gascoigne server stop", Stdout: []Check{Has("gascoigne server: stopped")}},
		{Run: "gascoigne server status", Stdout: []Check{Has("not running")}},
	}},
	Scenario{Name: "dashboard starts the server and prints its address", Slow: true, Server: true, Steps: []Step{
		{Run: "gascoigne dashboard", Stdout: []Check{Like(`^gascoigne dashboard: http://127\.0\.0\.1:\d+\n$`)}},
		{Run: "gascoigne server status", Stdout: []Check{Has("running on")}},
	}},
	Scenario{Name: "exec starts the server and uses it", Slow: true, Server: true, Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt", Stdout: []Check{ShorterThan("out.txt")}},
			{Run: "gascoigne server status", Stdout: []Check{Has("running on")}},
		}},
	Scenario{Name: "GASCOIGNE_SERVER=off never starts the server", Slow: true, Server: true,
		Files: map[string]string{"out.txt": failingGoTest},
		Env:   []string{"GASCOIGNE_SERVER=off"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt", Stdout: []Check{ShorterThan("out.txt")}},
			{Run: "gascoigne server status", Stdout: []Check{Has("not running")}},
		}},
)
