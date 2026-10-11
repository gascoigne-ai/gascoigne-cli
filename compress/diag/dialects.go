package diag

import "github.com/gascoigne-ai/gascoigne-cli/lazyre"

func re(s string) *lazyre.Regexp { return lazyre.New(s) }

var mavenNoise = `^\[INFO\] (Download|Scanning|---|\s*$|-+$|BUILD SUCCESS|Total time|Finished at|Using auto detected|Recompiling|Compiling \d+|skip non existing|\s+from pom|Building \S+ \S+$)|^Downloading from |^Downloaded from |^Progress \(`

// rules per build dialect; unknown dialects get no noise rules.
var rules = map[string]Rules{
	"rustc":    {Noise: re(`^\s+(Compiling|Checking|Downloaded|Downloading|Updating|Locking|Blocking|Fresh|Finished)\b`)},
	"javac":    {Noise: re(mavenNoise)},
	"maven":    {Noise: re(mavenNoise)},
	"dotnet":   {Noise: re(`^\s*(Determining projects to restore|All projects are up-to-date|Restored |MSBuild version|Time Elapsed|Workload updates)|^\s*\S+ -> /|^\s*\d+ Warning\(s\)$|^\s*\d+ Error\(s\)$`)},
	"bundler":  {Noise: re(`^(vite v[\d.]+ building|transforming\.\.\.|rendering chunks\.\.\.|computing gzip size\.\.\.)|✓ \d+ modules transformed|^\S+\s+[\d.]+ kB\s*│|^asset \S+ [\d.]+ \w?i?B|^(orphan|runtime|cacheable) modules|^\s+modules by path|^\s+\./\S+ [\d.]+ \w?i?B|^\s*\[built\]|^\s*\[code generated\]|^\s+at .*node_modules`)},
	"docker":   {Strip: re(`^#\d+ \d+\.\d+( |$)|^\d+\.\d+ `), Noise: re(`^#\d+ (\[internal\]|DONE|CACHED|resolve |transferring |naming |exporting |writing |sha256:|load build definition|load \.dockerignore|load metadata)|^#\d+ \[internal\]`)},
	"gcc":      {MaxFrame: 3},
	"clang":    {MaxFrame: 3},
	"go":       {},
	"tsc":      {},
	"eslint":   {},
	"ruff":     {},
	"mypy":     {},
	"golangci": {Noise: re(`^level=(info|debug)`)},
}

// For returns the rules of a build dialect.
func For(dialect string) Rules { return rules[dialect] }

// Dialects lists dialects with rules.
func Dialects() []string {
	var out []string
	for d := range rules {
		out = append(out, d)
	}
	return out
}
