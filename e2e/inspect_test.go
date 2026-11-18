package e2e

const calcSource = "package calc\n\n// Total sums the prices.\nfunc Total(prices []int) int {\n\tsum := 0\n\tfor _, price := range prices {\n\t\tsum += price\n\t}\n\treturn sum\n}\n\ntype Cart struct{}\n"

// Commands that read a file or the binary itself and print something about it.
var _ = group("inspect",
	Scenario{Name: "show lists a file's definitions with their line ranges", Files: map[string]string{"calc.go": calcSource},
		Steps: []Step{
			{Run: "gascoigne show calc.go", Stdout: []Check{
				Has("calc.go: 12 lines, 2 definitions", "func Total(prices []int) int {", "type Cart struct{}")}},
		}},
	Scenario{Name: "show file:symbol prints that symbol with line numbers", Files: map[string]string{"calc.go": calcSource},
		Steps: []Step{
			{Run: "gascoigne show calc.go:Total", Stdout: []Check{
				Has("calc.go:3-10 func Total", "3\t// Total sums the prices.", "9\t\treturn sum"), Lacks("type Cart")}},
		}},
	Scenario{Name: "show of a missing symbol is an error", Files: map[string]string{"calc.go": calcSource}, Steps: []Step{
		{Run: "gascoigne show calc.go:Nope", Exit: 1, Stdout: []Check{Empty()},
			Stderr: []Check{Has(`calc.go: symbol "Nope" not found`)}},
	}},
	Scenario{Name: "tokens counts a file per tokenizer family", Files: map[string]string{"in.txt": failingGoTest},
		Steps: []Step{
			{Run: "gascoigne tokens in.txt", Stdout: []Check{Has("970 bytes", "tokenizer", "claude", "o200k")}},
		}},
	Scenario{Name: "components lists the built-in components", Steps: []Step{
		{Run: "gascoigne components", Stdout: []Check{Has("kind", "receiver", "optimizer", "exporter", "extension")}},
	}},
)
