// Package npmoptimizer compresses npm/pnpm/yarn output: install noise and
// audit/funding boilerplate dropped, deprecations folded into one line,
// test runner output reduced to failures, TypeScript errors grouped by file.
package npmoptimizer

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	return p.Rewrite(func(b payload.Block) (string, error) {
		text := compress.StripANSI(b.Text)
		lines := compress.Lines(text)
		if b.Type == detect.NPM {
			lines = install(lines)
		}
		if compress.HasJSTest(text) {
			lines = compress.JSTest(lines)
		}
		return compress.Join(TSC(lines)), nil
	}, detect.NPM, detect.JSTest, detect.TSC)
}

var (
	noise = regexp.MustCompile("^npm (http|sill|silly|verb|verbose|timing|notice|info)\\b|" +
		"^\\d+ packages? (are|is) looking for funding|^\\s*run `npm fund` for details|" +
		"^To address (all )?issues|^\\s*npm audit fix|^Some issues need review|^a different dependency|" +
		"^Run `npm audit` for details")
	deprecated = regexp.MustCompile(`^npm (warn|WARN) deprecated (\S+): `)
)

// install drops progress, funding and audit boilerplate and folds
// deprecation warnings into one line; summaries and errors stay.
func install(lines []string) []string {
	var out, deps []string
	at := -1
	for _, l := range lines {
		if m := deprecated.FindStringSubmatch(l); m != nil {
			if at < 0 {
				at = len(out)
				out = append(out, "")
			}
			deps = append(deps, m[2])
			continue
		}
		if !noise.MatchString(l) {
			out = append(out, l)
		}
	}
	if at >= 0 {
		out[at] = fmt.Sprintf("npm warn deprecated (%d): %s", len(deps), strings.Join(deps, ", "))
	}
	return compress.SqueezeBlank(out)
}

var tscError = regexp.MustCompile(`^(\S+\.tsx?)\((\d+,\d+)\): (error|warning) (TS\d+: .*)$`)

// TSC groups "file.ts(l,c): error TSxxxx: msg" lines by file.
func TSC(lines []string) []string {
	var out []string
	file := ""
	for _, l := range lines {
		m := tscError.FindStringSubmatch(l)
		if m == nil {
			if file != "" && strings.HasPrefix(l, "  ") {
				out = append(out, "  "+l) // continuation of the previous error
				continue
			}
			file = ""
			out = append(out, l)
			continue
		}
		if m[1] != file {
			file = m[1]
			out = append(out, file+":")
		}
		out = append(out, fmt.Sprintf("  (%s) %s", m[2], m[4]))
	}
	return out
}
