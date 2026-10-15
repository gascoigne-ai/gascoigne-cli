package pcomb_test

import (
	"fmt"

	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

func ExamplePrefix() {
	m := pcomb.Prefix("Downloading from ", "Downloaded from ")
	fmt.Println(m("Downloading from central: https://repo"), m("[INFO] Building"))
	// Output: true false
}

func ExampleWord() {
	m := pcomb.Word("Compiling", "Checking")
	fmt.Println(m("Compiling foo v0.1.0"), m("Compilingly"))
	// Output: true false
}

func ExampleIndented() {
	m := pcomb.Indented(pcomb.Word("Finished"))
	fmt.Println(m("    Finished dev profile"), m("Finished dev profile"))
	// Output: true false
}

func ExampleTrimmed() {
	m := pcomb.Trimmed(pcomb.Prefix("Inspecting"))
	fmt.Println(m("Inspecting 3 files"), m("  Inspecting 3 files"), m("Done"))
	// Output: true true false
}

func ExampleAny() {
	m := pcomb.Any(pcomb.Prefix("warning:"), pcomb.Re(`^\d+ errors?$`))
	fmt.Println(m("warning: unused"), m("2 errors"), m("note: see above"))
	// Output: true true false
}

func ExampleRe() {
	m := pcomb.Re(`^\[\s*\d+ of \d+\] Compiling`)
	fmt.Println(m("[ 2 of 9] Compiling Main"), m("Compiling Main"))
	// Output: true false
}

func ExampleRules() {
	noise := pcomb.Rules{
		{Name: "cargo progress", Match: pcomb.Indented(pcomb.Word("Compiling", "Finished")),
			Yes: []string{"   Compiling foo v0.1.0"}, No: []string{"error: could not compile"}},
	}
	fmt.Println(noise.Verify())
	r, ok := noise.First("   Finished dev profile")
	fmt.Println(r.Name, ok)
	// Output:
	// <nil>
	// cargo progress true
}

func ExampleExact() {
	m := pcomb.Exact("Offenses:")
	fmt.Println(m("Offenses:"), m("Offenses: 3"))
	// Output: true false
}

func ExamplePrefixed() {
	m := pcomb.Prefixed("[INFO] ", pcomb.Prefix("Scanning", "Download"))
	fmt.Println(m("[INFO] Scanning for projects..."), m("[ERROR] Scanning"))
	// Output: true false
}
