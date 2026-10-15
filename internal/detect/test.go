package detect

import (
	"slices"
	"strings"
)

// Test runners.
const (
	GoTest     = "test/go"
	GoTestJSON = "test/go-json"
	JSTest     = "test/jest"
	Vitest     = "test/vitest"
	BunTest    = "test/bun"
	Mocha      = "test/mocha"
	DenoTest   = "test/deno"
	Pytest     = "test/pytest"
	CargoTest  = "test/cargo"
	RSpec      = "test/rspec"
	MavenTest  = "test/maven"
	DotnetTest = "test/dotnet"
	JUnit      = "test/junit"    // JUnit XML reports
	Coverage   = "test/coverage" // lcov tracefiles, go tool cover -func
)

func init() {
	register(
		Spec{Type: JUnit, Files: []string{"junit*.xml", "TEST-*.xml"}, Priority: 30, Refines: []string{"data/xml"},
			Sniff: func(t string, _ []string) bool {
				head := t[:min(len(t), 4096)]
				return strings.Contains(head, "<testsuite") && strings.Contains(t, "<testcase")
			}},
		Spec{Type: Coverage, Cmds: []string{"go tool cover -func"}, Files: []string{"lcov.info", "*.lcov"}, Priority: 47,
			Sniff: func(_ string, l []string) bool {
				return len(l) > 1 && (strings.HasPrefix(l[0], "TN:") || strings.HasPrefix(l[0], "SF:")) ||
					slices.ContainsFunc(l, func(s string) bool { return strings.HasPrefix(s, "total:\t") && strings.HasSuffix(s, "%") })
			}},
		Spec{Type: GoTestJSON, Cmds: []string{"go test -json"}, Priority: 10,
			Sniff: func(_ string, l []string) bool {
				return len(l) > 0 && containsAll(l[0], `"Action":`, `"Package":`)
			}},
		Spec{Type: GoTest, Cmds: []string{"go test"}, Priority: 50,
			Sniff: anyLine(`^=== RUN |^--- (FAIL|PASS|SKIP): |^ok  \t|^FAIL\t|^\?   \t.*\[no test files\]`)},
		Spec{Type: BunTest, Cmds: []string{"bun test"}, Priority: 53, Sniff: anyLine(`^bun test v\d`)},
		Spec{Type: Vitest, Cmds: []string{"vitest"}, Priority: 55, Sniff: anyLine(`^\s*RUN\s+v\d|^\s*Test Files\s+\d`)},
		Spec{Type: JSTest, Cmds: []string{"jest"}, Priority: 56,
			Sniff: anyLine(`^\s*(PASS|FAIL)\s+\S+\.(test|spec)\.\w+|^Tests:\s+\d`)},
		Spec{Type: Pytest, Cmds: []string{"pytest", "py.test"}, Priority: 48,
			Sniff: anyLine(`^=+ (test session starts|FAILURES|ERRORS|short test summary info) =+$|^=+ .*\d+ (passed|failed).* in [\d.]+s`)},
		Spec{Type: CargoTest, Cmds: []string{"cargo test", "cargo nextest"}, Priority: 48,
			Sniff: anyLine(`^test result: (ok|FAILED)\. \d+ passed`)},
		Spec{Type: Mocha, Cmds: []string{"mocha"}, Priority: 48, Sniff: anyLine(`^\s+\d+ passing \(\d+m?s\)`)},
		Spec{Type: RSpec, Cmds: []string{"rspec"}, Priority: 48, Sniff: anyLine(`^\d+ examples?, \d+ failures?`)},
		Spec{Type: MavenTest, Cmds: []string{"mvn test", "mvn verify", "mvnw test", "mvnw verify"}, Priority: 48,
			Sniff: anyLine(`^\[INFO\]  T E S T S|^\[(INFO|ERROR|WARNING)\] Tests run: \d+, Failures: \d+`)},
		Spec{Type: DotnetTest, Cmds: []string{"dotnet test"}, Priority: 48,
			Sniff: anyLine(`^(Passed|Failed)!\s+-\s+Failed:\s+\d+`)},
		Spec{Type: DenoTest, Cmds: []string{"deno test"}, Priority: 48,
			Sniff: anyLine(`^(ok|FAILED) \| \d+ passed|^running \d+ tests? from `)},
	)
}
