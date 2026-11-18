package e2e

// Two recorded runs from two agents in two sessions, for the reports to slice.
var twoAgents = []Step{
	{Run: "gascoigne exec -session s1 -agent a1 -- emit out.txt"},
	{Run: "gascoigne exec -session s2 -agent a2 -- emit out.txt"},
}

func withTwoAgents(steps ...Step) []Step { return append(append([]Step{}, twoAgents...), steps...) }

var _ = group("gain",
	Scenario{Name: "gain -json counts the recorded runs and the tokens saved", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: withTwoAgents(
			Step{Run: "gascoigne gain -json -all", Stdout: []Check{
				JSONField("total.runs", 2), JSONField("total.optimized", 2), Like(`"saved_tokens": [1-9]`)}}),
	},
	Scenario{Name: "gain -by agent -csv lists each agent", Files: map[string]string{"out.txt": failingGoTest},
		Steps: withTwoAgents(
			Step{Run: "gascoigne gain -by agent -csv -all", Stdout: []Check{
				Has("agent,runs,optimized,failed,in_tokens,out_tokens,saved_tokens,saved_pct", "a1,1,", "a2,1,")}}),
	},
	Scenario{Name: "gain -session and -agent filter the report", Files: map[string]string{"out.txt": failingGoTest},
		Steps: withTwoAgents(
			Step{Run: "gascoigne gain -json -session s2", Stdout: []Check{JSONField("total.runs", 1)}},
			Step{Run: "gascoigne gain -json -agent a1", Stdout: []Check{JSONField("total.runs", 1)}}),
	},
	Scenario{Name: "gain -oneline prints one line of totals", Files: map[string]string{"out.txt": failingGoTest},
		Steps: withTwoAgents(
			Step{Run: "gascoigne gain -oneline -all", Stdout: []Check{Like(`^gascoigne: \d+ tokens saved \(\d+%\)\n$`)}}),
	},
	Scenario{Name: "gain with an unknown grouping is an error", Steps: []Step{
		{Run: "gascoigne gain -by bogus", Exit: 1, Stderr: []Check{Has(`unknown grouping "bogus"`)}},
	}},
	Scenario{Name: "gain colors only when asked and never under NO_COLOR", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: withTwoAgents(
			Step{Run: "gascoigne gain -all", Stdout: []Check{Lacks("\x1b[")}},
			Step{Run: "gascoigne gain -all -color always", Stdout: []Check{Has("\x1b[")}}),
	},
	Scenario{Name: "GASCOIGNE_AGENT tags the runs with the agent name", Files: map[string]string{
		"out.txt": failingGoTest},
		Env: []string{"GASCOIGNE_AGENT=a9"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt"},
			{Run: "gascoigne gain -by agent -csv -all", Stdout: []Check{Has("a9,1,")}},
		}},
	Scenario{Name: "GASCOIGNE_MODEL tags the runs with the model name", Files: map[string]string{
		"out.txt": failingGoTest},
		Env: []string{"GASCOIGNE_MODEL=m1"},
		Steps: []Step{
			{Run: "gascoigne exec -- emit out.txt"},
			{Run: "gascoigne gain -by model -csv -all", Stdout: []Check{Has("m1,1,")}},
		}},
)
