package testreport

import (
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

// rules describe a runner's output for the generic engine. Every table is
// checked against its own examples by TestRulesVerify.
type rules struct {
	keep   pcomb.Rules // lines kept even when pass matches them (summaries, failures flagged on a pass-shaped line)
	pass   pcomb.Rules // a passing test or progress of passes: dropped and counted
	noise  pcomb.Rules // headers, banners, envelope lines: dropped
	frames pcomb.Rules // stack frames inside dependencies: dropped, counted
}

var (
	// npm and yarn wrap every JS runner in the same envelope lines.
	envelope = pcomb.Rules{
		{
			Name:  "npm notices",
			Match: pcomb.Prefix("npm notice ", "npm warn ", "npm info "),
			Yes:   []string{"npm warn deprecated inflight@1.0.6: This module is not supported", "npm notice New major version of npm available"},
			No:    []string{"npm ERR! code 1", "npm warn"},
		},
		{
			Name:  "package script echo",
			Match: pcomb.Re(`^> \S+@\S+ |^> \S+$`),
			Yes:   []string{"> app@1.0.0 test", "> jest"},
			No:    []string{"> jest --ci", ">"},
		},
		{
			Name:  "shell echo",
			Match: pcomb.Re(`^\$ \S+`),
			Yes:   []string{"$ jest --ci"},
			No:    []string{"$", "cost: $ 5"},
		},
		{
			Name:  "yarn banner",
			Match: pcomb.Prefix("yarn run v", "error Command failed", "info Visit https://yarnpkg"),
			Yes:   []string{"yarn run v1.22.19", "error Command failed with exit code 1.", "info Visit https://yarnpkg.com/en/docs/cli/run for documentation about this command."},
			No:    []string{"error Something broke", "yarn install v1.22.19"},
		},
		{
			Name:  "yarn done",
			Match: pcomb.Re(`^Done in [\d.]+s\.$`),
			Yes:   []string{"Done in 2.31s."},
			No:    []string{"Done in fast.", "Done in 2.31s"},
		},
	}

	// jsFrames are stack frames inside node internals and dependencies.
	jsFrames = pcomb.Rules{{
		Name:  "node frame",
		Match: pcomb.Re(`^\s*at .*(node_modules|node:internal|\(internal/)`),
		Yes:   []string{"    at Object.<anonymous> (/src/node_modules/jest-circus/build/run.js:12:5)", "    at process.processTicksAndRejections (node:internal/process/task_queues:95:5)", "    at Module._compile (internal/modules/cjs/loader.js:1085:14)"},
		No:    []string{"    at add (/src/math.js:3:9)", "    at add (/src/lib/node.js:3:9)"},
	}}

	// dots is a line of progress dots, one per passing test.
	dots = pcomb.Rules{{
		Name:  "progress dots",
		Match: pcomb.Re(`^\.+$`),
		Yes:   []string{"....", "."},
		No:    []string{"..F.", ""},
	}}

	// mavenNoise: maven progress and bookkeeping lines.
	mavenNoise = pcomb.Rules{
		{
			Name:  "downloads",
			Match: pcomb.Any(pcomb.Prefix("Downloading from ", "Downloaded from ", "Progress ("), pcomb.Prefixed("[INFO] ", pcomb.Prefix("Download"))),
			Yes:   []string{"Downloading from central: https://repo.maven.apache.org/maven2/a/a.pom", "Downloaded from central: https://repo.maven.apache.org/maven2/a/a.pom (4.1 kB at 12 kB/s)", "Progress (1): 4.1/12 kB", "[INFO] Downloading from central: https://repo.maven.apache.org/maven2/a/a.jar"},
			No:    []string{"[ERROR] Downloading failed", "Downloading"},
		},
		{
			Name:  "module header",
			Match: pcomb.Prefixed("[INFO] ", pcomb.Prefix("Building")),
			Yes:   []string{"[INFO] Building app 1.0-SNAPSHOT", "[INFO] Building jar: /src/target/app.jar"},
			No:    []string{"[ERROR] Building app 1.0", "Building app"},
		},
		{
			Name:  "separators",
			Match: pcomb.Prefixed("[INFO] ", pcomb.Any(pcomb.Prefix("---"), pcomb.Re(`^\s*$`), pcomb.Re(`^-+$`))),
			Yes:   []string{"[INFO] --- maven-surefire-plugin:3.2.2:test (default-test) @ app ---", "[INFO] ", "[INFO] ------------------------------------------------------------------------"},
			No:    []string{"[ERROR] ", "[INFO] x"},
		},
		{
			Name:  "build summary",
			Match: pcomb.Prefixed("[INFO] ", pcomb.Prefix("BUILD SUCCESS", "Total time", "Finished at")),
			Yes:   []string{"[INFO] BUILD SUCCESS", "[INFO] Total time:  2.314 s", "[INFO] Finished at: 2024-05-01T10:00:00+02:00"},
			No:    []string{"[INFO] BUILD FAILURE", "[ERROR] BUILD SUCCESS"},
		},
		{
			Name:  "compiler chatter",
			Match: pcomb.Prefixed("[INFO] ", pcomb.Any(pcomb.Prefix("Scanning", "Using auto detected", "Recompiling", "skip non existing"), pcomb.Re(`^Compiling \d+`), pcomb.Re(`^\s+from pom`))),
			Yes:   []string{"[INFO] Scanning for projects...", "[INFO] Using auto detected provider org.apache.maven.surefire.junit4.JUnit4Provider", "[INFO] Recompiling the module because of changed source code.", "[INFO] Compiling 12 source files to /src/target/classes", "[INFO] skip non existing resourceDirectory /src/src/test/resources", "[INFO]   from pom.xml"},
			No:    []string{"[INFO] Compiling sources", "[ERROR] Compiling 12 source files"},
		},
	}
)

// with joins rule tables into one.
func with(tables ...pcomb.Rules) pcomb.Rules { return slices.Concat(tables...) }

var dialects = map[string]rules{
	"jest": {
		pass: pcomb.Rules{
			{
				Name:  "passed file",
				Match: pcomb.Re(`^\s*PASS\s+\S`),
				Yes:   []string{"PASS src/math.test.js", " PASS  src/math.test.js (4.1 s)"},
				No:    []string{"FAIL src/math.test.js", "PASSED"},
			},
			{
				Name:  "check mark",
				Match: pcomb.Trimmed(pcomb.Prefix("✓", "✔", "√")),
				Yes:   []string{"    ✓ adds numbers (2 ms)"},
				No:    []string{"    ✕ subtracts numbers (3 ms)"},
			},
			{
				Name:  "file test count",
				Match: pcomb.Re(`^\s*\S+\.(test|spec)\.\w+\s+\(\d+ tests?\)`),
				Yes:   []string{"math.test.mjs (8 tests) 2ms"},
				No:    []string{"math.test.mjs failed"},
			},
		},
		noise: with(envelope, pcomb.Rules{{
			Name:  "run banner",
			Match: pcomb.Any(pcomb.Trimmed(pcomb.Prefix("Determining test suites")), pcomb.Prefix("Ran all test suites")),
			Yes:   []string{"Determining test suites to run...", "Ran all test suites."},
			No:    []string{"Ran 3 tests"},
		}}),
		frames: jsFrames,
	},
	"vitest": {
		pass: pcomb.Rules{
			{
				Name:  "check mark",
				Match: pcomb.Trimmed(pcomb.Prefix("✓ ", "✔ ", "√ ")),
				Yes:   []string{" ✓ adds numbers 1ms"},
				No:    []string{" ✓", " × subtracts 1ms"},
			},
			{
				Name:  "file test count",
				Match: pcomb.Re(`^\s*\S+\.(test|spec)\.\w+\s+\(\d+ tests?\)\s*\d*m?s?$`),
				Yes:   []string{"math.test.mjs (8 tests) 2ms", "math.test.mjs (1 test)"},
				No:    []string{"math.test.mjs (8 tests) failed"},
			},
		},
		noise: with(envelope, pcomb.Rules{
			{
				Name:  "run header",
				Match: pcomb.Re(`^\s*RUN\s+v\d`),
				Yes:   []string{" RUN  v3.2.7 /src"},
				No:    []string{"RUN tests"},
			},
			{
				Name:  "timing",
				Match: pcomb.Re(`^\s*(Start at|Duration)\s`),
				Yes:   []string{"   Start at  10:00:01", "   Duration  1.2s (transform 10ms, tests 3ms)"},
				No:    []string{"Duration"},
			},
		}),
		frames: jsFrames,
	},
	"bun": {
		pass: pcomb.Rules{{
			Name:  "pass marker",
			Match: pcomb.Trimmed(pcomb.Prefix("✓", "(pass)")),
			Yes:   []string{"(pass) adds numbers [0.12ms]", "✓ adds numbers"},
			No:    []string{"(fail) subtracts numbers [0.40ms]"},
		}},
		noise: with(envelope, pcomb.Rules{{
			Name:  "file header",
			Match: pcomb.Re(`^\S+\.(test|spec)\.\w+:$`),
			Yes:   []string{"math.test.ts:"},
			No:    []string{"math.test.ts: 3 pass"},
		}}),
		frames: jsFrames,
	},
	"mocha": {
		pass: pcomb.Rules{{
			Name:  "check mark",
			Match: pcomb.Indented(pcomb.Prefix("✓ ", "✔ ", "√ ")),
			Yes:   []string{"  ✓ adds numbers", "    ✔ adds numbers (3ms)"},
			No:    []string{"✓ adds numbers", "  1) subtracts numbers"},
		}},
		noise:  envelope,
		frames: jsFrames,
	},
	"deno": {
		pass: pcomb.Rules{{
			Name:  "ok result",
			Match: pcomb.Re(`\.\.\. ok \([\d.]+\s*(µs|us|ms|s)\)$`),
			Yes:   []string{"adds ... ok (12µs)", "adds ... ok (3ms)"},
			No:    []string{"subtracts ... FAILED (3ms)"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "test run header",
				Match: pcomb.Re(`^running \d+ tests? from `),
				Yes:   []string{"running 3 tests from ./math_test.ts", "running 1 test from ./math_test.ts"},
				No:    []string{"running tests"},
			},
			{
				Name:  "type check",
				Match: pcomb.Prefix("Check "),
				Yes:   []string{"Check file:///src/math_test.ts"},
				No:    []string{"Checking"},
			},
			{
				Name:  "download",
				Match: pcomb.Re(`^Download https?://`),
				Yes:   []string{"Download https://deno.land/std@0.224.0/assert/mod.ts"},
				No:    []string{"Downloaded https://deno.land/std/x.ts"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "runtime frame",
			Match: pcomb.Re(`^\s*at .*(ext:|deno:|https://deno\.land/std)`),
			Yes:   []string{"    at innerRun (ext:cli/40_test.js:100:7)", "    at assertEquals (https://deno.land/std@0.224.0/assert/assert_equals.ts:44:9)"},
			No:    []string{"    at add (file:///src/math.ts:3:9)"},
		}},
	},
	"pytest": {
		pass: pcomb.Rules{
			{
				Name:  "verbose pass",
				Match: pcomb.Re(`^\S+::\S+.* PASSED`),
				Yes:   []string{"tests/test_math.py::test_add PASSED                    [ 50%]"},
				No:    []string{"tests/test_math.py::test_sub FAILED                    [100%]"},
			},
			{
				Name:  "dots per file",
				Match: pcomb.Re(`^\S+\.py \.+\s*(\[\s*\d+%\])?$`),
				Yes:   []string{"tests/test_math.py ........                              [ 50%]", "tests/test_math.py ."},
				No:    []string{"tests/test_math.py F                                     [100%]"},
			},
		},
		noise: pcomb.Rules{
			{
				Name:  "session header",
				Match: pcomb.Prefix("platform ", "rootdir: ", "configfile: ", "plugins: ", "cachedir: ", "collecting "),
				Yes:   []string{"platform linux -- Python 3.13.0, pytest-8.3.3, pluggy-1.5.0", "rootdir: /src", "configfile: pyproject.toml", "plugins: cov-5.0.0", "cachedir: .pytest_cache", "collecting ... collected 9 items"},
				No:    []string{"E   assert 1 == 2"},
			},
			{
				Name:  "collection count",
				Match: pcomb.Re(`^collected \d+ items`),
				Yes:   []string{"collected 9 items"},
				No:    []string{"collected 1 item"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "dependency frame",
			Match: pcomb.Re(`site-packages/|/_pytest/|/pluggy/`),
			Yes:   []string{"/usr/lib/python3.13/site-packages/_pytest/python.py:1627: in runtest", "../.venv/lib/python3.13/site-packages/pluggy/_callers.py:121: in _multicall"},
			No:    []string{"tests/test_math.py:12: AssertionError"},
		}},
	},
	"cargo": {
		pass: pcomb.Rules{{
			Name:  "ok result",
			Match: pcomb.Re(`^test \S+ \.\.\. ok$`),
			Yes:   []string{"test math::tests::adds ... ok"},
			No:    []string{"test math::tests::subtracts ... FAILED", "test math::tests::slow ... ok <1s>"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "build progress",
				Match: pcomb.Indented(pcomb.Word("Compiling", "Downloaded", "Downloading", "Finished", "Running", "Fresh", "Blocking", "Updating", "Locking")),
				Yes:   []string{"   Compiling serde v1.0.210", "    Finished `test` profile [unoptimized + debuginfo] target(s) in 2.31s", "     Running unittests src/lib.rs (target/debug/deps/app-4f3a)"},
				No:    []string{"Compiling serde v1.0.210", "   Compilingly"},
			},
			{
				Name:  "test count",
				Match: pcomb.Re(`^running \d+ tests?$`),
				Yes:   []string{"running 3 tests", "running 1 test"},
				No:    []string{"running 3 tests from x"},
			},
		},
		frames: pcomb.Rules{
			{
				Name:  "std backtrace frame",
				Match: pcomb.Re(`^\s+\d+: (std::|core::|test::|<alloc::)`),
				Yes:   []string{"   3: core::panicking::panic_fmt", "  12: std::rt::lang_start::{{closure}}", "   9: test::run_test"},
				No:    []string{"   4: app::math::divide", "3: core::panicking::panic_fmt"},
			},
			{
				Name:  "rustc source frame",
				Match: pcomb.Indented(pcomb.Prefix("at /rustc/")),
				Yes:   []string{"             at /rustc/90b35a6239c3d8bdabc530a6a0816f7ff89a0aaf/library/core/src/panicking.rs:72:14"},
				No:    []string{"             at ./src/math.rs:7:5"},
			},
		},
	},
	"rspec": {
		pass: dots,
		noise: pcomb.Rules{{
			Name:  "run header",
			Match: pcomb.Prefix("Randomized with seed", "Run options:"),
			Yes:   []string{"Randomized with seed 41921", "Run options: --seed 41921"},
			No:    []string{"Failed examples:"},
		}},
		frames: pcomb.Rules{{
			Name:  "gem frame",
			Match: pcomb.Re(`/gems/|/rubygems/|/bundler/`),
			Yes:   []string{"# /usr/lib/ruby/gems/3.3.0/gems/rspec-core-3.13.0/lib/rspec/core/example.rb:263:in 'block in run'", "# /usr/lib/ruby/3.3.0/rubygems/core_ext/kernel_require.rb:136:in 'require'", "# /usr/lib/ruby/3.3.0/bundler/setup.rb:10:in 'load'"},
			No:    []string{"# ./spec/user_spec.rb:12:in 'block (2 levels) in <top (required)>'"},
		}},
	},
	"maven": {
		pass: pcomb.Rules{{
			Name:  "passing class",
			Match: pcomb.Re(`^\[INFO\] Tests run: \d+, Failures: 0, Errors: 0, Skipped: \d+, Time elapsed`),
			Yes:   []string{"[INFO] Tests run: 3, Failures: 0, Errors: 0, Skipped: 0, Time elapsed: 0.04 s - in com.x.MathTest"},
			No:    []string{"[ERROR] Tests run: 3, Failures: 1, Errors: 0, Skipped: 0, Time elapsed: 0.04 s", "[INFO] Tests run: 3, Failures: 0, Errors: 0, Skipped: 0"},
		}},
		noise: mavenNoise,
		frames: pcomb.Rules{{
			Name:  "test framework frame",
			Match: pcomb.Indented(pcomb.Prefix("at org.junit", "at org.apache.maven", "at org.opentest4j", "at sun.reflect", "at java.base/", "at jdk.internal", "at java.lang.reflect")),
			Yes:   []string{"\tat org.junit.jupiter.api.Assertions.assertEquals(Assertions.java:1)", "\tat java.base/java.lang.reflect.Method.invoke(Method.java:580)", "\tat org.opentest4j.AssertionFailedError.<init>(AssertionFailedError.java:42)"},
			No:    []string{"\tat com.x.MathTest.adds(MathTest.java:9)", "at org.junit.Assert.fail(Assert.java:1)"},
		}},
	},
	"ctest": {
		pass: pcomb.Rules{{
			Name:  "passed test",
			Match: pcomb.Re(`^\s*\d+/\d+ Test\s+#\d+: .*\.\.+\s*(\*\*\*)?Passed`),
			Yes:   []string{"1/3 Test #1: math_add ........................   Passed    0.01 sec", "2/3 Test #2: math_sub ....................***Passed    0.01 sec"},
			No:    []string{"3/3 Test #3: math_div ....................***Failed    0.01 sec"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "test start",
				Match: pcomb.Re(`^\s+Start\s+\d+:`),
				Yes:   []string{"      Start 1: math_add"},
				No:    []string{"Start testing: Nov 01 10:00 UTC"},
			},
			{
				Name:  "run banner",
				Match: pcomb.Any(pcomb.Prefix("Test project ", "Constructing a list of tests", "Updating test list", "Checking test dependency", "Start testing:", "End testing:"), pcomb.Re(`^Added \d+ tests`)),
				Yes:   []string{"Test project /src/build", "Constructing a list of tests", "Updating test list for fixtures", "Added 3 tests to list of tests to run", "Checking test dependency graph...", "Start testing: Nov 01 10:00 UTC", "End testing: Nov 01 10:00 UTC"},
				No:    []string{"The following tests FAILED:"},
			},
		},
	},
	"gtest": {
		pass: pcomb.Rules{{
			Name:  "ok result",
			Match: pcomb.Prefix("[       OK ]"),
			Yes:   []string{"[       OK ] MathTest.Adds (0 ms)"},
			No:    []string{"[  FAILED  ] MathTest.Subtracts (0 ms)"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "test start",
				Match: pcomb.Prefix("[ RUN      ]", "[----------]"),
				Yes:   []string{"[ RUN      ] MathTest.Adds", "[----------] 3 tests from MathTest", "[----------] Global test environment set-up."},
				No:    []string{"[  FAILED  ] MathTest.Subtracts"},
			},
			{
				Name:  "run totals",
				Match: pcomb.Any(pcomb.Prefix("[==========] Running"), pcomb.Re(`^\[==========\] \d+ tests? from \d+ test suites? ran`)),
				Yes:   []string{"[==========] Running 3 tests from 1 test suite.", "[==========] 3 tests from 1 test suite ran. (1 ms total)"},
				No:    []string{"[  PASSED  ] 3 tests."},
			},
		},
	},
	"catch": {noise: pcomb.Rules{
		{
			Name:  "run header",
			Match: pcomb.Prefix("Randomness seeded to:", "Filters:"),
			Yes:   []string{"Randomness seeded to: 2431872339", "Filters: [math]"},
			No:    []string{"FAILED:"},
		},
		{
			Name:  "tilde rule",
			Match: pcomb.Re(`^~+$`),
			Yes:   []string{"~~~~~~~~~~~~~~~~~~~~"},
			No:    []string{"~~ x ~~"},
		},
	}},
	"sbt": {
		keep: pcomb.Rules{{
			Name:  "failed test",
			Match: pcomb.Re(`\*\*\* (FAILED|ABORTED) \*\*\*`),
			Yes:   []string{"[info] - should subtract *** FAILED ***", "[info] MathSpec *** ABORTED ***"},
			No:    []string{"[info] - should add"},
		}},
		pass: pcomb.Rules{{
			Name:  "passed test",
			Match: pcomb.Re(`^\[info\] +- .+$`),
			Yes:   []string{"[info] - should add", "[info]   - should nest"},
			No:    []string{"[info] MathSpec:", "[info] -"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "sbt startup",
				Match: pcomb.Prefixed("[info] ", pcomb.Prefix("welcome to sbt", "loading ", "set current project", "done compiling", "Updating", "Resolving", "Fetched", "resolving key")),
				Yes:   []string{"[info] welcome to sbt 1.10.2 (Eclipse Adoptium Java 21)", "[info] loading project definition from /src/project", "[info] set current project to app (in build file:/src/)", "[info] done compiling", "[info] Updating", "[info] Resolving org.scala-lang#scala-library;2.13.14 ...", "[info] Fetched artifacts of", "[info] resolving key references (15112 settings) ..."},
				No:    []string{"[error] loading failed", "[info] MathSpec:"},
			},
			{
				Name:  "compile count",
				Match: pcomb.Re(`^\[info\] compiling \d+ `),
				Yes:   []string{"[info] compiling 2 Scala sources to /src/target/classes ..."},
				No:    []string{"[info] compiling sources"},
			},
			{
				Name:  "total time",
				Match: pcomb.Prefix("[success] Total time"),
				Yes:   []string{"[success] Total time: 4 s, completed Nov 1, 2026, 10:00:00 AM"},
				No:    []string{"[success] done"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "library frame",
			Match: pcomb.Re(`^\[(info|error)\]\s+at (org\.scalatest|scala\.|java\.base|jdk\.internal|sbt\.|cats\.effect|zio\.|akka\.)`),
			Yes:   []string{"[info]   at org.scalatest.Assertions.newAssertionFailedException(Assertions.scala:472)", "[error]   at java.base/java.lang.Thread.run(Thread.java:1583)"},
			No:    []string{"[info]   at app.MathSpec.$anonfun$new$2(MathSpec.scala:9)"},
		}},
	},
	"dub": {
		noise: pcomb.Rules{
			{
				Name:  "build steps",
				Match: pcomb.Prefix(`Performing "`, "Building ", "Running ", "Linking", "Up-to-date"),
				Yes:   []string{`Performing "unittest" build using /usr/bin/dmd for x86_64.`, "Building app ~master: building configuration [unittest]", "Running ./app-test-library", "Linking...", "Up-to-date app ~master: target for configuration is up to date."},
				No:    []string{"core.exception.AssertError@source/app.d(12): Assertion failure"},
			},
			{
				Name:  "package fetching",
				Match: pcomb.Prefix("Fetching", "Placing", "Selecting"),
				Yes:   []string{"Fetching silly 1.1.1 (getting package)", "Placing silly 1.1.1 to /home/u/.dub/packages/", "Selecting silly 1.1.1"},
				No:    []string{"Failed to fetch"},
			},
		},
		frames: pcomb.Rules{
			{
				Name:  "druntime frame",
				Match: pcomb.Re(`^(#\d+ +)?0x[0-9a-f]+ in (_?D3std|_D4core|_Dmain|rt\.dmain2)`),
				Yes:   []string{"#4   0x55d0c3a1b2c4 in _D4core9exception__T14__switchErrorZ", "0x55d0c3a1b2c4 in _Dmain"},
				No:    []string{"#4   0x55d0c3a1b2c4 in _D3app4mainFZv"},
			},
			{
				Name:  "unresolved frame",
				Match: pcomb.Prefix("??:?"),
				Yes:   []string{"??:? _d_run_main [0x55d0c3a1b2c4]"},
				No:    []string{"source/app.d:12"},
			},
		},
	},
	"phpunit": {
		pass: pcomb.Rules{
			{
				Name:  "progress dots",
				Match: pcomb.Re(`^\.+(\s+\d+ / \d+ \(\d+%\))?$`),
				Yes:   []string{"...........                                                       11 / 11 (100%)", "...."},
				No:    []string{"..F.", ".... 3 / 11"},
			},
			{
				Name:  "pass marker",
				Match: pcomb.Re(`^\s+(PASS|✓)\s`),
				Yes:   []string{"   PASS  Tests\\Unit\\MathTest", "   ✓ adds numbers"},
				No:    []string{"   FAIL  Tests\\Unit\\MathTest", "PASS  Tests"},
			},
		},
		noise: pcomb.Rules{
			{
				Name:  "version banner",
				Match: pcomb.Re(`^PHPUnit \d`),
				Yes:   []string{"PHPUnit 11.3.1 by Sebastian Bergmann and contributors."},
				No:    []string{"PHPUnit Failures"},
			},
			{
				Name:  "environment",
				Match: pcomb.Prefix("Runtime:", "Configuration:", "Random Seed:"),
				Yes:   []string{"Runtime:       PHP 8.3.10", "Configuration: /src/phpunit.xml", "Random Seed:   1730454000"},
				No:    []string{"Time: 00:00.012, Memory: 8.00 MB"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "vendor frame",
			Match: pcomb.Re(`^#\d+ .*vendor/(phpunit|pestphp|symfony|laravel)/`),
			Yes:   []string{"#1 /src/vendor/phpunit/phpunit/src/Framework/TestCase.php(1234): PHPUnit\\Framework\\TestCase->runTest()"},
			No:    []string{"#0 /src/tests/MathTest.php(12): App\\Math->div(1, 0)"},
		}},
	},
	"swift": {
		pass: pcomb.Rules{{
			Name:  "passed case",
			Match: pcomb.Re(`^Test Case '.*' passed \(`),
			Yes:   []string{"Test Case '-[AppTests.MathTests testAdd]' passed (0.001 seconds)."},
			No:    []string{"Test Case '-[AppTests.MathTests testSub]' failed (0.002 seconds)."},
		}},
		noise: pcomb.Rules{
			{
				Name:  "case start",
				Match: pcomb.Re(`^Test Case '.*' started\.`),
				Yes:   []string{"Test Case '-[AppTests.MathTests testAdd]' started."},
				No:    []string{"Test Case '-[AppTests.MathTests testAdd]' passed (0.001 seconds)."},
			},
			{
				Name:  "build progress",
				Match: pcomb.Any(pcomb.Prefix("Building for", "Compiling", "Build complete"), pcomb.Re(`^\[\d+/\d+\]`)),
				Yes:   []string{"Building for debugging...", "Compiling App Math.swift", "[5/7] Compiling App Math.swift", "Build complete! (2.31s)"},
				No:    []string{"error: build had 1 command failures"},
			},
			{
				Name:  "suite start and pass",
				Match: pcomb.Re(`^Test Suite '.*' (started|passed) at`),
				Yes:   []string{"Test Suite 'All tests' started at 2026-11-01 10:00:00.001", "Test Suite 'MathTests' passed at 2026-11-01 10:00:00.003."},
				No:    []string{"Test Suite 'MathTests' failed at 2026-11-01 10:00:00.003."},
			},
		},
	},
	"mix": {
		pass: dots,
		noise: pcomb.Rules{
			{
				Name:  "seed",
				Match: pcomb.Prefix("Randomized with seed", "Running ExUnit with seed"),
				Yes:   []string{"Randomized with seed 123456", "Running ExUnit with seed: 123456, max_cases: 8"},
				No:    []string{"Finished in 0.03 seconds"},
			},
			{
				Name:  "compile output",
				Match: pcomb.Re(`^Compiling \d+ files|^Generated \S+ app`),
				Yes:   []string{"Compiling 2 files (.ex)", "Generated app app"},
				No:    []string{"Compiling lib/app.ex"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "elixir frame",
			Match: pcomb.Indented(pcomb.Prefix("(ex_unit) ", "(elixir) ")),
			Yes:   []string{"    (ex_unit) lib/ex_unit/runner.ex:1: x/1", "    (elixir) lib/enum.ex:2: Enum.each/2"},
			No:    []string{"    test/app_test.exs:7: (test)"},
		}},
	},
	"minitest": {
		pass: dots,
		noise: pcomb.Rules{{
			Name:  "run header",
			Match: pcomb.Prefix("Run options:", "# Running:", "Started with run options"),
			Yes:   []string{"Run options: --seed 1234", "# Running:", "Started with run options --seed 1234"},
			No:    []string{"Finished in 0.002s"},
		}},
		frames: pcomb.Rules{{
			Name:  "gem frame",
			Match: pcomb.Re(`/gems/|/rubygems/`),
			Yes:   []string{"/usr/lib/ruby/gems/3.3.0/gems/minitest-5.25.1/lib/minitest/test.rb:94:in 'block in run'", "/usr/lib/ruby/3.3.0/rubygems/core_ext/kernel_require.rb:136:in 'require'"},
			No:    []string{"test/user_test.rb:12:in 'test_name'"},
		}},
	},
	"unittest": {
		pass: pcomb.Rules{
			dots[0],
			{
				Name:  "ok suffix",
				Match: pcomb.Re(`\.\.\. ok$`),
				Yes:   []string{"test_add (test_math.MathTest.test_add) ... ok"},
				No:    []string{"test_sub (test_math.MathTest.test_sub) ... FAIL"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "dependency frame",
			Match: pcomb.Re(`site-packages/|/unittest/`),
			Yes:   []string{"  File \"/usr/lib/python3.13/unittest/case.py\", line 58, in testPartExecutor"},
			No:    []string{"  File \"/src/test_math.py\", line 12, in test_sub"},
		}},
	},
	"tap": {
		pass: pcomb.Rules{
			{
				Name:  "ok line",
				Match: pcomb.Re(`^\s*ok \d+ - `),
				Yes:   []string{"ok 1 - adds numbers", "    ok 2 - nested"},
				No:    []string{"not ok 3 - subtracts numbers"},
			},
			{
				Name:  "check with timing",
				Match: pcomb.Re(`^\s*(✔|✓) .*\([\d.]+ms\)$`),
				Yes:   []string{"✔ adds numbers (0.51ms)"},
				No:    []string{"✔ adds numbers"},
			},
			{
				Name:  "subtest header",
				Match: pcomb.Trimmed(pcomb.Prefix("# Subtest:")),
				Yes:   []string{"# Subtest: adds numbers", "    # Subtest: nested"},
				No:    []string{"# pass 3"},
			},
		},
		noise: pcomb.Rules{
			{
				Name:  "version",
				Match: pcomb.Prefix("TAP version"),
				Yes:   []string{"TAP version 13"},
				No:    []string{"1..3"},
			},
			{
				Name:  "yaml block",
				Match: pcomb.Trimmed(pcomb.Exact("---", "...")),
				Yes:   []string{"  ---", "  ..."},
				No:    []string{"  --- x"},
			},
			{
				Name:  "yaml timing",
				Match: pcomb.Trimmed(pcomb.Prefix("duration_ms:", "type: 'test'")),
				Yes:   []string{"  duration_ms: 0.51", "  type: 'test'"},
				No:    []string{"  error: 'boom'"},
			},
		},
		frames: jsFrames,
	},
	"playwright": {
		pass: pcomb.Rules{{
			Name:  "passed test",
			Match: pcomb.Re(`^\s+✓\s+\d+ \[`),
			Yes:   []string{"  ✓  1 [chromium] › tests/home.spec.ts:3:1 › has title (1.2s)"},
			No:    []string{"  ✘  2 [chromium] › tests/home.spec.ts:9:1 › has footer (1.0s)"},
		}},
		noise: pcomb.Rules{{
			Name:  "run header",
			Match: pcomb.Re(`^Running \d+ tests? using \d+ workers?`),
			Yes:   []string{"Running 3 tests using 2 workers", "Running 1 test using 1 worker"},
			No:    []string{"Running tests"},
		}},
		frames: pcomb.Rules{{
			Name:  "playwright frame",
			Match: pcomb.Re(`node_modules/(@?playwright|playwright-core)`),
			Yes:   []string{"    at /src/node_modules/playwright-core/lib/client/frame.js:12:5", "    at /src/node_modules/@playwright/test/lib/index.js:5:1"},
			No:    []string{"    at /src/tests/home.spec.ts:9:3"},
		}},
	},
	"cypress": {
		pass: pcomb.Rules{{
			Name:  "check mark",
			Match: pcomb.Indented(pcomb.Prefix("✓ ")),
			Yes:   []string{"    ✓ shows the title (212ms)"},
			No:    []string{"✓ shows the title", "    1) shows the footer"},
		}},
		noise: pcomb.Rules{{
			Name:  "run start",
			Match: pcomb.Trimmed(pcomb.Prefix("(Run Starting)")),
			Yes:   []string{"  (Run Starting)"},
			No:    []string{"  (Run Finished)"},
		}},
		frames: pcomb.Rules{{
			Name:  "runner frame",
			Match: pcomb.Re(`^\s+at .*(cypress_runner|node_modules)`),
			Yes:   []string{"      at runnable.fn (http://localhost:3000/__cypress/runner/cypress_runner.js:1:2)"},
			No:    []string{"      at Context.eval (webpack:///./cypress/e2e/home.cy.js:5:8)"},
		}},
	},
	"karma": {
		pass: pcomb.Rules{{
			Name:  "success progress",
			Match: pcomb.Re(`: Executed \d+ of \d+ SUCCESS`),
			Yes:   []string{"Chrome Headless 126.0.0 (Linux x86_64): Executed 3 of 3 SUCCESS (0.02 secs / 0.01 secs)"},
			No:    []string{"Chrome Headless 126.0.0 (Linux x86_64): Executed 3 of 3 (1 FAILED)"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "info log",
				Match: pcomb.Re(`^\d+ \d\d \d{4} \d\d:\d\d:\d\d\.\d+:INFO`),
				Yes:   []string{"01 11 2026 10:00:00.123:INFO [karma-server]: Karma v6.4.4 server started at http://localhost:9876/"},
				No:    []string{"01 11 2026 10:00:00.123:ERROR [karma]: boom"},
			},
			{
				Name:  "blank line",
				Match: pcomb.Trimmed(pcomb.Exact("")),
				Yes:   []string{"", "   "},
				No:    []string{"x"},
			},
		},
		frames: jsFrames,
	},
	"dart": {
		keep: pcomb.Rules{
			{
				Name:  "final verdict",
				Match: pcomb.Re(`All tests passed|Some tests failed`),
				Yes:   []string{"00:02 +3: All tests passed!", "00:02 +2 -1: Some tests failed."},
				No:    []string{"00:01 +2: test/math_test.dart: adds"},
			},
			{
				Name:  "error marker",
				Match: pcomb.Re(`\[E\]`),
				Yes:   []string{"00:01 +1 -1: test/math_test.dart: subtracts [E]"},
				No:    []string{"00:01 +1: test/math_test.dart: subtracts"},
			},
		},
		pass: pcomb.Rules{{
			Name:  "progress line",
			Match: pcomb.Re(`^\d\d:\d\d \+\d+(?: ~\d+)?: `),
			Yes:   []string{"00:01 +1: test/math_test.dart: adds", "00:01 +1 ~1: test/math_test.dart: skipped"},
			No:    []string{"Error: boom"},
		}},
		noise: pcomb.Rules{{
			Name:  "pub setup",
			Match: pcomb.Prefix("Resolving dependencies", "Got dependencies", "Building package executable"),
			Yes:   []string{"Resolving dependencies...", "Got dependencies!", "Building package executable... (1.2s)"},
			No:    []string{"Because app depends on x, version solving failed."},
		}},
	},
	"hspec": {
		pass: pcomb.Rules{{
			Name:  "check mark",
			Match: pcomb.Re(`\[✔\]$`),
			Yes:   []string{"    adds numbers [✔]"},
			No:    []string{"    subtracts numbers [✘]"},
		}},
		noise: pcomb.Rules{{
			Name:  "cabal setup",
			Match: pcomb.Prefix("Building ", "Configuring ", "Preprocessing ", "Resolving dependencies", "Build profile", "In order", "Up to date"),
			Yes:   []string{"Building test suite 'spec' for app-0.1.0.0..", "Configuring app-0.1.0.0...", "Preprocessing test suite 'spec' for app-0.1.0.0..", "Resolving dependencies...", "Build profile: -w ghc-9.8.2 -O1", "In order, the following will be built (use -v for more details):", "Up to date"},
			No:    []string{"app.hs:12:1: error:"},
		}},
	},
	"dotnet": {
		pass: pcomb.Rules{{
			Name:  "passed test",
			Match: pcomb.Re(`^\s+Passed \S+`),
			Yes:   []string{"  Passed MathTests.Adds [3 ms]"},
			No:    []string{"  Failed MathTests.Subtracts [3 ms]", "Passed MathTests.Adds"},
		}},
		noise: pcomb.Rules{
			{
				Name:  "restore chatter",
				Match: pcomb.Trimmed(pcomb.Prefix("Determining projects to restore", "All projects are up-to-date", "Restored ", "Starting test execution")),
				Yes:   []string{"  Determining projects to restore...", "  All projects are up-to-date for restore.", "  Restored /src/App/App.csproj (in 312 ms).", "Starting test execution, please wait..."},
				No:    []string{"  Restoring"},
			},
			{
				Name:  "test file count",
				Match: pcomb.Re(`^\s*A total of \d+ test files`),
				Yes:   []string{"A total of 1 test files matched the specified pattern."},
				No:    []string{"A total of tests"},
			},
			{
				Name:  "output path",
				Match: pcomb.Re(`^\s*\S+ -> /`),
				Yes:   []string{"  App -> /src/App/bin/Debug/net8.0/App.dll"},
				No:    []string{"App -> bin/Debug/App.dll"},
			},
		},
		frames: pcomb.Rules{{
			Name:  "runtime frame",
			Match: pcomb.Indented(pcomb.Prefix("at System.", "at Xunit.", "at NUnit.", "at Microsoft.VisualStudio")),
			Yes:   []string{"   at System.Linq.Enumerable.First[TSource](IEnumerable`1 source)", "   at Xunit.Assert.Equal[T](T expected, T actual)"},
			No:    []string{"   at App.Tests.MathTests.Subtracts() in /src/MathTests.cs:line 12"},
		}},
	},
}
