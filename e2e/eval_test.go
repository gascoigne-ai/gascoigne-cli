package e2e

// `eval` measures savings on a fixture set: a cases file, the fixtures it names, goldens and a
// baseline next to it. These scenarios use a one-case copy so they never touch the repo's own.
const (
	evalCase    = "- file: go/test-fail.txt\n  attrs: { cmd: go test ./... }\n  expect_type: test/go\n  keep: [\"--- FAIL: TestReserve\"]\n  min_saved: 40\n"
	evalBadCase = "- file: go/test-fail.txt\n  attrs: { cmd: go test ./... }\n  expect_type: test/go\n  keep: [\"a line the output never had\"]\n  min_saved: 40\n"
)

var _ = group("eval",
	Scenario{Name: "eval -update writes the golden and baseline, and the next run passes", Slow: true,
		Files: map[string]string{"set/go/test-fail.txt": failingGoTest, "set/cases.yaml": evalCase},
		Steps: []Step{
			{Run: "gascoigne eval -cases set/cases.yaml -update", Files: map[string][]Check{
				"${work}/set/baseline.tsv": {Has("go/test-fail")}}},
			{Run: "gascoigne eval -cases set/cases.yaml", Stdout: []Check{Has("total"), Lacks("FAIL")}},
		}},
	Scenario{Name: "eval fails when a fact the output must keep is lost", Slow: true,
		Files: map[string]string{"set/go/test-fail.txt": failingGoTest, "set/cases.yaml": evalBadCase},
		Steps: []Step{
			{Run: "gascoigne eval -cases set/cases.yaml", Exit: 1, Stderr: []Check{Has(`lost "a line the output never had"`)}},
		}},
)
