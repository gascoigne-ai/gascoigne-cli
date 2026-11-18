package e2e

// Every optimized output keeps its original in the ledger, and `raw` hands it back.
// `emit -exit 1 out.txt` is the recorded command, so its failure key is "cmd:emit -exit 1 out.txt".
var failingRun = Step{
	Run:    "gascoigne exec -session s1 -- emit -exit 1 out.txt",
	Exit:   1,
	Save:   map[string]string{"id": `\[raw:([0-9a-f]+)\]`},
	Stdout: []Check{ShorterThan("out.txt")},
}

var _ = group("ledger",
	Scenario{Name: "raw returns the original byte-exact", Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne raw ${id}", Stdout: []Check{SameAsFile("out.txt")}},
		}},
	Scenario{Name: "a partial raw read (-lines, -grep, -query) does not mark the source as failing",
		Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne raw ${id} -lines 1-2", Stdout: []Check{Has("example.com/shop")}},
			{Run: "gascoigne raw ${id} -grep FAIL", Stdout: []Check{Has("--- FAIL: TestReserve")}},
			{Run: "gascoigne raw ${id} -query reserve", Exit: 0},
			{Run: "gascoigne gain -failures", Stdout: []Check{Lacks("emit -exit 1 out.txt")}},
		}},
	Scenario{Name: "a full raw read marks the source as failing", Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne raw ${id}"},
			{Run: "gascoigne gain -failures", Stdout: []Check{Has("cmd:emit -exit 1 out.txt", "raw_requested")}},
		}},
	Scenario{Name: "fail marks a source with a reason and forget clears it", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne fail ${id} -reason lost_the_stack"},
			{Run: "gascoigne gain -failures", Stdout: []Check{Has("cmd:emit -exit 1 out.txt", "lost_the_stack")}},
			{Run: "gascoigne forget 'cmd:emit -exit 1 out.txt'"},
			{Run: "gascoigne gain -failures", Stdout: []Check{Lacks("cmd:emit -exit 1 out.txt")}},
		}},
	Scenario{Name: "flush deletes the originals but keeps the recorded runs", Files: map[string]string{
		"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne flush", Stdout: []Check{Has("deleted 1 stored originals")}},
			{Run: "gascoigne raw ${id}", Exit: 1, Stderr: []Check{Has("not found")}},
			{Run: "gascoigne gain -json -session s1", Stdout: []Check{JSONField("total.runs", 1)}},
		}},
	Scenario{Name: "reset deletes the runs of one session only", Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne exec -session s2 -- emit out.txt"},
			{Run: "gascoigne reset -session s1", Stdout: []Check{Has("deleted 1 runs")}},
			{Run: "gascoigne gain -json -all", Stdout: []Check{JSONField("total.runs", 1)}},
		}},
	Scenario{Name: "reset -all cannot be combined with a filter", Steps: []Step{
		{Run: "gascoigne reset -all -session s1", Exit: 1, Stdout: []Check{Empty()},
			Stderr: []Check{Has("cannot be filtered")}},
	}},
	Scenario{Name: "reset takes its filters from flags, never from GASCOIGNE_* variables",
		Files: map[string]string{"out.txt": failingGoTest},
		Steps: []Step{
			failingRun,
			{Run: "gascoigne reset", Env: []string{"GASCOIGNE_SESSION=other"}, Stdout: []Check{Has("deleted 1 runs")}},
		}},
)
