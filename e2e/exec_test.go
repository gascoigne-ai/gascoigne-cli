package e2e

const failingGoTest = "@testdata/eval/go/test-fail.txt"

// `exec -- <cmd>` runs a command, prints its optimized output and keeps its exit code.
// `emit` is the stand-in tool: it prints files and exits with the code it is given.
var _ = group("exec",
	Scenario{Name: "exec passes the child's exit code through", Steps: []Step{
		{Run: "gascoigne exec -- emit", Exit: 0},
		{Run: "gascoigne exec -- emit -exit 1", Exit: 1},
		{Run: "gascoigne exec -- emit -exit 7", Exit: 7},
	}},
	Scenario{Name: "exec merges the child's stderr into its output", Files: map[string]string{
		"out.txt": "to stdout\n", "err.txt": "to stderr\n"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit -stderr err.txt out.txt",
				Stdout: []Check{Has("to stdout", "to stderr")}, Stderr: []Check{Lacks("to stderr")}},
		}},
	Scenario{Name: "exec shortens a failing test run and names the original", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne exec -- emit -exit 1 out.txt", Exit: 1,
				Stdout: []Check{ShorterThan("out.txt"), Has("--- FAIL: TestTotal", "[raw:")}},
		}},
	Scenario{Name: "exec -raw prints the output verbatim", Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne exec -raw -- emit out.txt", Stdout: []Check{SameAsFile("out.txt")}},
		}},
	Scenario{Name: "GASCOIGNE_RAW=1 prints the output verbatim", Files: map[string]string{"out.txt": failingGoTest},
		Env: []string{"GASCOIGNE_RAW=1"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt", Stdout: []Check{SameAsFile("out.txt")}},
		}},
	Scenario{Name: "exec without a command is an error", Steps: []Step{
		{Run: "gascoigne exec", Exit: 1, Stdout: []Check{Empty()},
			Stderr: []Check{Has("gascoigne: exec: want a command")}},
	}},
	Scenario{Name: "exec of a missing program exits 127", Steps: []Step{
		{Run: "gascoigne exec -- nosuchprogram", Exit: 127, Stderr: []Check{Has("executable file not found")}},
	}},
)
