package testreport

import (
	"cmp"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// rules describe a runner's output for the generic engine.
type rules struct {
	keep   *lazyre.Regexp // lines kept even when pass matches them (summaries, failures flagged on a pass-shaped line)
	pass   *lazyre.Regexp // a passing test or progress of passes: dropped and counted
	noise  *lazyre.Regexp // headers, banners, envelope lines: dropped
	frames *lazyre.Regexp // stack frames inside dependencies: dropped, counted
}

func re(s string) *lazyre.Regexp { return lazyre.New(s) }

var (
	envelope = `^npm (notice|warn|info) |^> \S+@\S+ |^> \S+$|^\$ \S+|^yarn run v|^Done in [\d.]+s\.$|^error Command failed|^info Visit https://yarnpkg`
	jsFrames = re(`^\s*at .*(node_modules|node:internal|\(internal/)`)
	// decoration lines carry no text: ⎯⎯⎯⎯⎯[1/2]⎯⎯⎯, ─────, ===== (pytest headers have text).
	decoration = re(`^[\s⎯─━═=_*~-]*(\[\d+/\d+\])?[\s⎯─━═=_*~-]*$`)
)

// never matches; a rule a runner has no use for.
var never = re(`[^\s\S]`)

var dialects = map[string]rules{
	"jest": {
		pass:   re(`^\s*(PASS\s+\S|✓|✔|√)|^\s*\S+\.(test|spec)\.\w+\s+\(\d+ tests?\)`),
		noise:  re(envelope + `|^\s*Determining test suites|^Ran all test suites`),
		frames: jsFrames,
	},
	"vitest": {
		pass:   re(`^\s*(✓|✔|√)\s|^\s*\S+\.(test|spec)\.\w+\s+\(\d+ tests?\)\s*\d*m?s?$`),
		noise:  re(envelope + `|^\s*RUN\s+v\d|^\s*Start at\s|^\s*Duration\s`),
		frames: jsFrames,
	},
	"bun": {
		pass:   re(`^\s*(✓|\(pass\))`),
		noise:  re(envelope + `|^\S+\.(test|spec)\.\w+:$`),
		frames: jsFrames,
	},
	"mocha": {
		pass:   re(`^\s+(✓|✔|√)\s`),
		noise:  re(envelope),
		frames: jsFrames,
	},
	"deno": {
		pass:   re(`\.\.\. ok \([\d.]+\s*(µs|us|ms|s)\)$`),
		noise:  re(`^running \d+ tests? from |^Check |^Download https?://`),
		frames: re(`^\s*at .*(ext:|deno:|https://deno\.land/std)`),
	},
	"pytest": {
		pass:   re(`^\S+::\S+.* PASSED|^\S+\.py \.+\s*(\[\s*\d+%\])?$`),
		noise:  re(`^(platform |rootdir: |configfile: |plugins: |cachedir: |collecting |collected \d+ items)`),
		frames: re(`site-packages/|/_pytest/|/pluggy/`),
	},
	"cargo": {
		pass:   re(`^test \S+ \.\.\. ok$`),
		noise:  re(`^\s+(Compiling|Downloaded|Downloading|Finished|Running|Fresh|Blocking|Updating|Locking)\b|^running \d+ tests?$`),
		frames: re(`^\s+\d+: (std::|core::|test::|<alloc::)|^\s+at /rustc/`),
	},
	"rspec": {
		pass:   re(`^\.+$`),
		noise:  re(`^Randomized with seed|^Run options:`),
		frames: re(`/gems/|/rubygems/|/bundler/`),
	},
	"maven": {
		pass:   re(`^\[INFO\] Tests run: \d+, Failures: 0, Errors: 0, Skipped: \d+, Time elapsed`),
		noise:  re(`^\[INFO\] (Download|Building|Scanning|---|\s*$|-+$|BUILD SUCCESS|Total time|Finished at|Using auto detected|Recompiling|Compiling \d+|skip non existing|\s+from pom)|^Downloading from |^Downloaded from |^Progress \(`),
		frames: re(`^\s+at (org\.junit|org\.apache\.maven|org\.opentest4j|sun\.reflect|java\.base/|jdk\.internal|java\.lang\.reflect)`),
	},
	"ctest": {
		pass:  re(`^\s*\d+/\d+ Test\s+#\d+: .*\.\.+\s*(\*\*\*)?Passed`),
		noise: re(`^\s+Start\s+\d+:|^(Test project |Constructing a list of tests|Updating test list|Added \d+ tests|Checking test dependency|Start testing:|End testing:)`),
	},
	"gtest": {
		noise: re(`^\[ RUN      \]|^\[----------\]|^\[==========\] Running|^\[==========\] \d+ tests? from \d+ test suites? ran`),
		pass:  re(`^\[       OK \]`),
	},
	"catch": {noise: re(`^Randomness seeded to:|^Filters:|^~+$`)},
	"sbt": {
		keep:   re(`\*\*\* FAILED \*\*\*|\*\*\* ABORTED \*\*\*`),
		pass:   re(`^\[info\] +- .+$`),
		noise:  re(`^\[info\] (welcome to sbt|loading |set current project|compiling \d+ |done compiling|Updating|Resolving|Fetched|resolving key)|^\[success\] Total time`),
		frames: re(`^\[(info|error)\]\s+at (org\.scalatest|scala\.|java\.base|jdk\.internal|sbt\.|cats\.effect|zio\.|akka\.)`),
	},
	"dub": {
		noise:  re(`^(Performing "|Building |Running |Linking|Up-to-date|Fetching|Placing|Selecting)`),
		frames: re(`^(#\d+ +)?0x[0-9a-f]+ in (_?D3std|_D4core|_Dmain|rt\.dmain2)|^\?\?:\?`),
	},
	"phpunit": {
		pass:   re(`^\.+(\s+\d+ / \d+ \(\d+%\))?$|^\s+(PASS|✓)\s`),
		noise:  re(`^(PHPUnit \d|Runtime:|Configuration:|Random Seed:)`),
		frames: re(`^#\d+ .*vendor/(phpunit|pestphp|symfony|laravel)/`),
	},
	"swift": {
		pass:  re(`^Test Case '.*' passed \(`),
		noise: re(`^Test Case '.*' started\.|^(Building for|Compiling|\[\d+/\d+\]|Build complete)|^Test Suite '.*' (started|passed) at`),
	},
	"mix": {
		pass:   re(`^\.+$`),
		noise:  re(`^Randomized with seed|^Compiling \d+ files|^Generated \S+ app|^Running ExUnit with seed`),
		frames: re(`^\s+\((ex_unit|elixir)\) `),
	},
	"minitest": {
		pass:   re(`^\.+$`),
		noise:  re(`^Run options:|^# Running:|^Started with run options`),
		frames: re(`/gems/|/rubygems/`),
	},
	"unittest": {
		pass:   re(`^\.+$|\.\.\. ok$`),
		frames: re(`site-packages/|/unittest/`),
	},
	"tap": {
		pass:   re(`^\s*ok \d+ - |^\s*(✔|✓) .*\([\d.]+ms\)$|^\s*# Subtest:`),
		noise:  re(`^TAP version|^\s*---$|^\s*\.\.\.$|^\s*duration_ms:|^\s*type: 'test'`),
		frames: jsFrames,
	},
	"playwright": {
		pass:   re(`^\s+✓\s+\d+ \[`),
		noise:  re(`^Running \d+ tests? using \d+ workers?`),
		frames: re(`node_modules/(@?playwright|playwright-core)`),
	},
	"cypress": {
		pass:   re(`^\s+✓ `),
		noise:  re(`^\s*\(Run Starting\)`),
		frames: re(`^\s+at .*(cypress_runner|node_modules)`),
	},
	"karma": {
		pass:   re(`: Executed \d+ of \d+ SUCCESS`),
		noise:  re(`^\d+\s+\d\d \d\d:\d\d:\d\d\.\d+:INFO|^\s*$`),
		frames: jsFrames,
	},
	"dart": {
		keep:  re(`All tests passed|Some tests failed|\[E\]`),
		pass:  re(`^\d\d:\d\d \+\d+(?: ~\d+)?: `),
		noise: re(`^Resolving dependencies|^Got dependencies|^Building package executable`),
	},
	"hspec": {
		pass:  re(`\[✔\]$`),
		noise: re(`^(Building |Configuring |Preprocessing |Resolving dependencies|Build profile|In order|Up to date)`),
	},
	"dotnet": {
		pass:   re(`^\s+Passed \S+`),
		noise:  re(`^\s*(Determining projects to restore|All projects are up-to-date|Restored |Starting test execution|A total of \d+ test files)|^\s*\S+ -> /`),
		frames: re(`^\s+at (System\.|Xunit\.|NUnit\.|Microsoft\.VisualStudio)`),
	},
}

// Dialects lists runners Compress understands besides go.
func Dialects() []string {
	out := []string{"go", "go-json"}
	for d := range dialects {
		out = append(out, d)
	}
	return out
}

// Compress applies a runner's rules: passes counted and dropped, noise and
// dependency frames dropped, everything else (failures, diffs, summaries) kept.
func Compress(dialect string, lines []string) ([]string, bool) {
	r, ok := dialects[dialect]
	if !ok {
		return lines, false
	}
	r.keep, r.pass, r.noise, r.frames = cmp.Or(r.keep, never), cmp.Or(r.pass, never), cmp.Or(r.noise, never), cmp.Or(r.frames, never)
	var out []string
	passes, frames := 0, 0
	flush := func() {
		if frames > 0 {
			out = append(out, compress.Omitted(frames, "dependency stack frames"))
			frames = 0
		}
	}
	for _, l := range lines {
		switch {
		case r.keep.MatchString(l):
			flush()
			out = append(out, l)
		case r.pass.MatchString(l):
			passes++
		case r.frames.MatchString(l):
			frames++
		case r.noise.MatchString(l), strings.TrimSpace(l) != "" && decoration.MatchString(l):
		default:
			flush()
			out = append(out, l)
		}
	}
	flush()
	out = compress.SqueezeBlank(trimBlankEnds(out))
	if passes > 0 {
		out = append(out, compress.Omitted(passes, "passing test lines"))
	}
	return out, true
}

func trimBlankEnds(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
