package compress

import "github.com/lukaszb/gascoigne/lazyre"

var jsTestOutput = lazyre.New(`(?m)^\s*(PASS|FAIL)\s+\S+\.(test|spec)\.\w+|^Tests:\s+\d|^\s*Test Files\s+\d|^\s*\d+ (pass|fail)$`)

// HasJSTest reports whether text contains jest, vitest or bun test output.
func HasJSTest(text string) bool { return jsTestOutput.MatchString(text) }
