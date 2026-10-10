package detect

// Test runners.
const (
	GoTest     = "test/go"
	GoTestJSON = "test/go-json"
	JSTest     = "test/jest" // jest, vitest
	BunTest    = "test/bun"
)

func init() {
	register(
		Spec{Type: GoTestJSON, Cmds: []string{"go test -json"}, Priority: 10,
			Sniff: func(_ string, l []string) bool {
				return len(l) > 0 && containsAll(l[0], `"Action":`, `"Package":`)
			}},
		Spec{Type: GoTest, Cmds: []string{"go test"}, Priority: 50,
			Sniff: anyLine(`^=== RUN |^--- (FAIL|PASS|SKIP): |^ok  \t|^FAIL\t|^\?   \t.*\[no test files\]`)},
		Spec{Type: BunTest, Cmds: []string{"bun test"}, Priority: 53, Sniff: anyLine(`^bun test v\d`)},
		Spec{Type: JSTest, Cmds: []string{"jest", "vitest"}, Priority: 56,
			Sniff: anyLine(`^\s*(PASS|FAIL)\s+\S+\.(test|spec)\.\w+|^Tests:\s+\d|^\s*Test Files\s+\d`)},
	)
}
