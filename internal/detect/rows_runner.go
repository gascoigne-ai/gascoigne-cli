package detect

// Test runners declared as rows; rules per dialect are in compress/testreport.
func init() {
	registerRows(
		Row{Type: "test/ctest", Priority: 58, Cmds: []string{"ctest"}, Cmd: "ctest --output-on-failure",
			Sniff: `^\d+% tests passed, \d+ tests? failed out of \d+|^Test project \S+`, Sample: "Test project /src/build\n    Start 1: unit\n1/1 Test #1: unit ......   Passed    0.01 sec\n\n100% tests passed, 0 tests failed out of 1\n"},
		Row{Type: "test/gtest", Priority: 58, Sniff: `^\[==========\] Running \d+ tests? from`, Sample: "[==========] Running 2 tests from 1 test suite.\n[ RUN      ] Cart.Add\n[       OK ] Cart.Add (0 ms)\n"},
		Row{Type: "test/catch", Priority: 58, Sniff: `^Randomness seeded to: \d+|^All tests passed \(\d+ assertions? in \d+ test cases?\)|^\[doctest\] doctest version`, Sample: "Randomness seeded to: 123\nAll tests passed (4 assertions in 2 test cases)\n"},
		Row{Type: "test/sbt", Priority: 58, Cmds: []string{"sbt test", "sbt testOnly", "scala-cli test", "mill test"}, Cmd: "sbt test",
			Sniff: `^\[info\] (All tests passed|Tests: succeeded \d+, failed \d+|Run completed in)`, Sample: "[info] Run completed in 1 second.\n[info] All tests passed.\n"},
		Row{Type: "test/dub", Priority: 58, Cmds: []string{"dub test"}, Cmd: "dub test",
			Sniff: `^\d+ modules? passed unittests|core\.exception\.AssertError@`, Sample: "core.exception.AssertError@source/app.d(12): Assertion failure\n"},
		Row{Type: "test/phpunit", Priority: 58, Cmds: []string{"phpunit", "pest", "php artisan test"}, Cmd: "phpunit tests",
			Sniff: `^PHPUnit \d+\.\d+|^\s+(PASS|FAIL)\s+Tests\\`, Sample: "PHPUnit 10.5.2 by Sebastian Bergmann and contributors.\n"},
		Row{Type: "test/swift", Priority: 58, Cmds: []string{"swift test"}, Cmd: "swift test",
			Sniff: `^Test Suite '.*' (started|passed|failed) at`, Sample: "Test Suite 'All tests' started at 2026-10-14 10:00:00.000\n"},
		Row{Type: "test/mix", Priority: 58, Cmds: []string{"mix test"}, Cmd: "mix test",
			Sniff: `^Running ExUnit with seed:|^Finished in [\d.]+ seconds? \(`, Sample: "Running ExUnit with seed: 1, max_cases: 8\nFinished in 0.1 seconds (0.0s async, 0.1s sync)\n"},
		Row{Type: "test/minitest", Priority: 58, Cmds: []string{"rake test", "rails test"}, Cmd: "rake test",
			Sniff: `^# Running:$|^Finished in [\d.]+s, [\d.]+ runs/s`, Sample: "Run options: --seed 1\n\n# Running:\n\n..\n\nFinished in 0.01s, 100 runs/s\n"},
		Row{Type: "test/unittest", Priority: 58, Cmds: []string{"unittest", "nosetests", "nose2"}, Cmd: "python -m unittest discover",
			Sniff: `^Ran \d+ tests? in [\d.]+s$`, Sample: "..\nRan 2 tests in 0.001s\n\nOK\n"},
		Row{Type: "test/tap", Priority: 58, Cmds: []string{"node --test", "tap", "ava"}, Cmd: "node --test",
			Sniff: `^TAP version 1[34]`, Sample: "TAP version 13\nok 1 - adds\n1..1\n"},
		Row{Type: "test/playwright", Priority: 58, Cmds: []string{"playwright test"}, Cmd: "npx playwright test",
			Sniff: `^Running \d+ tests? using \d+ workers?`, Sample: "Running 3 tests using 2 workers\n"},
		Row{Type: "test/cypress", Priority: 58, Cmds: []string{"cypress run"}, Cmd: "npx cypress run",
			Sniff: `^\s+\(Run Starting\)`, Sample: "  (Run Starting)\n"},
		Row{Type: "test/karma", Priority: 58, Cmds: []string{"karma start", "ng test"}, Cmd: "ng test",
			Sniff: `^(Headless)?(Chrome|Firefox|Safari)[\w .]* \(.*\): Executed \d+ of \d+`, Sample: "Chrome Headless 120.0 (Linux x86_64): Executed 1 of 3 SUCCESS (0 secs / 0.01 secs)\n"},
		Row{Type: "test/dart", Priority: 58, Cmds: []string{"flutter test", "dart test"}, Cmd: "flutter test",
			Sniff: `^\d\d:\d\d \+\d+(?: ~\d+)?(?: -\d+)?: `, Sample: "00:01 +0: loading test/a_test.dart\n00:02 +1: All tests passed!\n"},
		Row{Type: "test/hspec", Priority: 58, Cmds: []string{"cabal test", "stack test"}, Cmd: "stack test"},
	)
}
