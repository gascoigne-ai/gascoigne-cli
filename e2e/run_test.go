package e2e

// `run` optimizes stdin to stdout, with no child process.
var _ = group("run",
	Scenario{Name: "run shortens a failing test log read from stdin", Files: map[string]string{
		"in.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne run", Stdin: failingGoTest,
				Stdout: []Check{ShorterThan("in.txt"), Has("--- FAIL: TestTotal")}},
		}},
	Scenario{Name: "run -raw passes stdin through byte-exact", Files: map[string]string{"in.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne run -raw", Stdin: failingGoTest, Stdout: []Check{SameAsFile("in.txt")}},
		}},
	Scenario{Name: "run leaves a short input alone", Steps: []Step{
		{Run: "gascoigne run", Stdin: "hi\n", Stdout: []Check{Is("hi\n")}},
	}},
)
