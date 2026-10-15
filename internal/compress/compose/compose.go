// Package compose compresses a section of mixed output (a task runner's
// task, a docker build step, a CI step) with the engine of whatever family
// it turns out to be: test, build or pkg. Used by composite families.
package compose

import (
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/diag"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/pkgnoise"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/testreport"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// Lines detects the section's type from its command (cmd may be empty) or
// content and compresses it; unknown content only loses repeats.
func Lines(lines []string, cmd string) []string {
	typ := detect.Detect(compress.Join(lines), detect.Hints{Cmd: cmd})
	return As(typ, lines)
}

// As compresses lines as the given type.
func As(typ string, lines []string) []string {
	dialect := detect.Dialect(typ)
	switch detect.Family(typ) {
	case detect.FamilyTest:
		switch dialect {
		case "go":
			return testreport.Go(lines)
		case "go-json":
			if out, err := testreport.GoJSON(compress.Join(lines)); err == nil {
				return out
			}
		default:
			if out, ok := testreport.Compress(dialect, lines); ok {
				return out
			}
		}
	case detect.FamilyBuild:
		return diag.Compress(lines, diag.For(dialect))
	case detect.FamilyPkg:
		if r, ok := pkgnoise.For(dialect); ok {
			return pkgnoise.Compress(lines, r)
		}
	}
	return compress.SqueezeBlank(compress.DedupeRuns(lines))
}
