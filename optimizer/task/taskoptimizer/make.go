package taskoptimizer

import (
	"regexp"

	"github.com/lukaszb/gascoigne/compress/compose"
)

var (
	makeNoise = regexp.MustCompile(`^(g?make|just)(\[\d+\])?: (Entering|Leaving) directory|^(g?make)(\[\d+\])?: Nothing to be done for`)
	makeOwn   = regexp.MustCompile(`^(g?make|just)(\[\d+\])?: |^(g?make|just) `)
)

// makeLike: make and just run tools inline. Directory chatter goes, their
// own status lines stay at the end, and the tools' output is compressed as
// whatever it is (gcc, go, tests…).
func makeLike(lines []string) []string {
	var tool, own []string
	for _, l := range lines {
		switch {
		case makeNoise.MatchString(l):
		case makeOwn.MatchString(l):
			own = append(own, l)
		default:
			tool = append(tool, l)
		}
	}
	return append(compose.Lines(tool, ""), own...)
}
