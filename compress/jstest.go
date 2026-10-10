package compress

import (
	"strings"

	"github.com/lukaszb/gascoigne/lazyre"
)

var (
	// Passing test/file lines of jest, vitest and bun test.
	jsPass = lazyre.New(`^\s*(PASS\s+\S|✓|✔|√|\(pass\))|^\s*\S+\.(test|spec)\.\w+\s+\(\d+ tests?\)`)
	// Stack frames inside dependencies say nothing about the failure.
	jsDepFrame = lazyre.New(`^\s*at .*(node_modules|node:internal|\(internal/)`)
	// Per-file headers of bun test ("src/a.test.ts:") are noise once passes are gone.
	bunFileHeader = lazyre.New(`^\S+\.(test|spec)\.\w+:$`)
)

// JSTest compresses jest/vitest/bun test output: drops passing tests and
// dependency stack frames, keeps failures, assertion diffs and summaries.
func JSTest(lines []string) []string {
	var out []string
	passes, frames := 0, 0
	flush := func() {
		if frames > 0 {
			out = append(out, Omitted(frames, "dependency stack frames"))
			frames = 0
		}
	}
	for _, l := range lines {
		switch {
		case jsPass.MatchString(l):
			passes++
		case jsDepFrame.MatchString(l):
			frames++
		case bunFileHeader.MatchString(strings.TrimSpace(l)):
		default:
			flush()
			out = append(out, l)
		}
	}
	flush()
	out = SqueezeBlank(out)
	if passes > 0 {
		out = append(out, Omitted(passes, "passing test lines"))
	}
	return out
}
