// Package watch knows build tools in watch mode (tsc -w, webpack --watch and
// its dev server, vite/rollup/esbuild builds, cargo watch): their output is a
// series of cycles, one per rebuild, each ending with a result line. Only the
// last cycle says what is broken now.
package watch

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// endRe matches the line that ends a cycle.
var endRe = lazyre.New(strings.Join([]string{
	`Found \d+ errors?\. Watching for file changes\.`,                                 // tsc
	`^webpack \S+ compiled\b`,                                                         // webpack 5
	`(?i)^\s*(webpack: )?compiled (successfully|with \d+ (errors?|warnings?)|failed)`, // webpack-dev-server, next
	`^built in \d+(\.\d+)?m?s\.?$`,                                                    // vite build --watch
	`waiting for changes\.\.\.$`,                                                      // rollup -w
	`^\[watch\] build finished`,                                                       // esbuild --watch
	`^\[Finished running\. Exit status: \d+\]`,                                        // cargo watch
}, "|"))

// End reports whether line ends a watch cycle.
func End(line string) bool { return endRe.MatchString(compress.StripANSI(line)) }

const omittedSuffix = ", results:"

// maxResults caps the earlier cycles' result lines kept.
const maxResults = 5

// Last keeps the last complete cycle and anything after it (a rebuild in
// progress); earlier cycles become one marker followed by their latest
// result lines. Fewer than two cycles stay as they are.
func Last(lines []string) []string {
	if len(lines) > 0 && strings.HasSuffix(lines[0], omittedSuffix) {
		return lines // already cut
	}
	var ends []int
	for i, l := range lines {
		if End(l) {
			ends = append(ends, i)
		}
	}
	if len(ends) < 2 {
		return lines
	}
	start := ends[len(ends)-2] + 1 // the last complete cycle begins after the one before
	earlier := ends[:len(ends)-1]
	out := []string{fmt.Sprintf("… %d earlier watch cycles omitted (%d lines)%s", len(earlier), start, omittedSuffix)}
	for _, i := range earlier[max(0, len(earlier)-maxResults):] {
		out = append(out, "  "+strings.TrimSpace(compress.StripANSI(lines[i])))
	}
	return append(out, lines[start:]...)
}
