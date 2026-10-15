package diag

import "github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"

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

	"cmake":   {Noise: re(`^-- (The (C|CXX|ASM|CUDA) compiler|Detecting|Check for|Looking for|Performing Test|Found |Configuring done|Generating done|Build files have been written|Using )|^\[\s*\d+%\] (Building|Linking|Built target|Generating|Scanning|Automatic)|^Scanning dependencies of target|^g?make\[\d+\]: (Entering|Leaving)`)},
	"ninja":   {Noise: re(`^\[\d+/\d+\] (Building|Linking|Generating|Running|Install|Re-running|Creating|Scanning)`)},
	"meson":   {Noise: re(`^(Run-time dependency|Library|Program|Dependency|Compiler for|Build targets in project|Found ninja|Message:|Project (name|version)|Host machine|Build machine|Has header|Checking for|Configuring)\b|^\[\d+/\d+\] (Compiling|Linking|Generating)`)},
	"bazel":   {Noise: re(`^(\(\d\d:\d\d:\d\d\) )?(Loading:|Analyzing:|INFO: (Analyzed|Found \d|From |Build options|Invocation|Elapsed|Streaming|Current date|Reading|Repository|Writing)|DEBUG:|\[[\d,]+ / [\d,]+\]|Computing main repo mapping)|^\s*Fetching`)},
	"cpplint": {Noise: re(`^\d+ warnings? generated\.|^Suppressed \d+ warnings|^Use -header-filter|^Use -system-headers|^Checking \S+|^\d+/\d+ files checked`), MaxFrame: 3},
	"msvc":    {Noise: re(`^Microsoft \(R\)|^Copyright \(C\) Microsoft|^\S+\.(cpp|cc|c|cxx)\s*$|^Generating Code|^Finished Generating|^\s+Creating library`)},
	"dmd":     {Noise: re(`^(Performing "|Starting Performing|Building \S+ \S+: building configuration|Running (ldc2|dmd)|Linking|Compiling|\s*Up-to-date|Fetching|Placing|Using |Selecting|Basic dub)`)},
	"scala": {
		Tag:      re(`^\[(error|warn)\] `),
		Noise:    re(`^\[info\] (welcome to sbt|loading |set current project|compiling \d+ |done compiling|Updating|Resolving|Fetched|resolving key|Non-compiled module|Compilation completed)|^\[success\] `),
		MaxFrame: 6,
	},
	"swift":   {Noise: re(`^(\[\d+/\d+\] (Compiling|Emitting|Write|Linking)|Building for |Build complete|Compiling |Fetching |Computing version|Creating working|Resolving|Fetched|Cloning)`)},
	"zig":     {Noise: re(`^\s*\+- `)},
	"kotlin":  {},
	"php":     {},
	"rubocop": {Noise: re(`^Inspecting \d+ files?|^[.CWEFR]+$|^Offenses:$`)},
	"pylint":  {Noise: re(`^\*{5,} Module `)},
	"lint":    {},
	"dart":    {Noise: re(`^Analyzing \S+\.\.\.`)},
	"elixir":  {Noise: re(`^==> |^Compiling \d+ files? \(\.ex\)|^Generated \S+ app`)},
	"haskell": {Noise: re(`^\[\s*\d+ of \d+\] Compiling|^Linking |^Loaded package|^Preprocessing |^Building |^Configuring |^In order, the following|^Resolving dependencies|^Build profile|^Up to date|^Downloading|^Starting|^Completed`), MaxFrame: 10},
	"fmt":     {},
	"generic": {},
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
