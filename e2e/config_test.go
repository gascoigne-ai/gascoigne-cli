package e2e

// Tuning: flags beat environment variables, which beat the config file. The `test` optimizer is
// what shortens the failing Go test run, so enabling or disabling it shows in the output size.
var _ = group("config",
	Scenario{Name: "config prints the effective config with -set applied", Steps: []Step{
		{Run: "gascoigne config -set optimizers.grep.max_per_file=50", Stdout: []Check{Has("max_per_file: 50")}},
	}},
	Scenario{Name: "-set beats GASCOIGNE_SET", Env: []string{"GASCOIGNE_SET=optimizers.grep.max_per_file=10"},
		Steps: []Step{
			{Run: "gascoigne config", Stdout: []Check{Has("max_per_file: 10")}},
			{Run: "gascoigne config -set optimizers.grep.max_per_file=50", Stdout: []Check{Has("max_per_file: 50"), Lacks("max_per_file: 10")}},
		}},
	Scenario{Name: "-config beats GASCOIGNE_CONFIG", Files: map[string]string{
		"env.yaml":  "version: 1\noptimizers:\n  grep:\n    max_per_file: 11\n",
		"flag.yaml": "version: 1\noptimizers:\n  grep:\n    max_per_file: 22\n"},
		Env: []string{"GASCOIGNE_CONFIG=${work}/env.yaml"},
		Steps: []Step{
			{Run: "gascoigne config", Stdout: []Check{Has("max_per_file: 11")}},
			{Run: "gascoigne config -config flag.yaml", Stdout: []Check{Has("max_per_file: 22")}},
		}},
	Scenario{Name: "-disable stops an optimizer and exec prints the output verbatim", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt", Stdout: []Check{ShorterThan("out.txt")}},
			{Run: "gascoigne exec -disable test -- emit out.txt", Stdout: []Check{SameAsFile("out.txt")}},
			{Run: "gascoigne exec -- emit out.txt", Env: []string{"GASCOIGNE_DISABLE=test"}, Stdout: []Check{SameAsFile("out.txt")}},
		}},
	Scenario{Name: "-only beats GASCOIGNE_ONLY", Files: map[string]string{"out.txt": failingGoTest},
		Env: []string{"GASCOIGNE_ONLY=grep"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt", Stdout: []Check{SameAsFile("out.txt")}},
			{Run: "gascoigne exec -only test -- emit out.txt", Stdout: []Check{ShorterThan("out.txt")}},
		}},
	Scenario{Name: "an unknown optimizer name is an error", Steps: []Step{
		{Run: "gascoigne exec -disable nosuchoptimizer -- emit", Exit: 1,
			Stderr: []Check{Has(`optimizer "nosuchoptimizer" not defined`)}},
	}},
	Scenario{Name: "GASCOIGNE_LEDGER decides where the ledger is written", Files: map[string]string{
		"out.txt": failingGoTest},
		Env: []string{"GASCOIGNE_LEDGER=${work}/custom.db"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt"},
			{Run: "gascoigne gain -oneline", Stdout: []Check{Has("tokens saved")},
				Files: map[string][]Check{"${work}/custom.db": {Like(`(?s).`)}}},
		}},
)
