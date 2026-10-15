package detect

// Build tools declared as rows; rules per dialect are in compress/diag.
func init() {
	// Any compiler or linter nobody listed (before grep, whose shape it shares): several lines in the file:line: error shape.
	register(Spec{Type: "build/generic", Priority: 69,
		Sniff: atLeast(`^[^\s:]+\.\w+:\d+(:\d+)?: (fatal error|error|warning)\b|^\S+\(\d+(,\d+)?\): (error|warning)\b`, 3)})
	registerRows(
		Row{Type: "build/cmake", Cmds: []string{"cmake"}, Cmd: "cmake -S . -B build",
			Sniff: `^CMake (Error|Warning)`, Sample: "CMake Error at CMakeLists.txt:12 (find_package):\n  Could not find a package configuration file provided by \"Foo\"\n"},
		Row{Type: "build/ninja", Cmds: []string{"ninja"}, Cmd: "ninja -C build",
			Sniff: `^\[\d+/\d+\] (Building|Linking|Generating|Running|Install)|^FAILED: `, Sample: "[1/3] Building CXX object a.o\nFAILED: a.o\n"},
		Row{Type: "build/meson", Cmds: []string{"meson"}, Cmd: "meson compile -C build",
			Sniff: `^The Meson build system$`, Sample: "The Meson build system\nVersion: 1.3.0\nSource dir: /src\n"},
		Row{Type: "build/bazel", Cmds: []string{"bazel build", "bazelisk build", "bazel run"}, Cmd: "bazel build //app:main",
			Sniff: `^(\(\d\d:\d\d:\d\d\) )?(INFO: Invocation ID|INFO: (Analyzed|Build completed)|ERROR: .*BUILD:\d+)`, Sample: "INFO: Analyzed target //app:main (0 packages loaded).\nINFO: Build completed successfully, 3 total actions\n"},
		Row{Type: "build/cpplint", Cmds: []string{"clang-tidy", "run-clang-tidy", "cppcheck", "cpplint"}, Cmd: "clang-tidy src/a.cc -p build"},
		Row{Type: "build/fmt", Cmds: []string{"rustfmt", "cargo fmt", "black", "prettier", "clang-format", "cargo doc"}, Cmd: "cargo fmt --check",
			Sniff: `^Diff in \S+:\d+:|^would reformat |^\d+ files? would be reformatted|^\[warn\] Code style issues`, Sample: "would reformat src/a.py\nOh no!\n1 file would be reformatted.\n"},
		Row{Type: "build/msvc", Cmds: []string{"cl", "cl.exe"}, Cmd: "cl /EHsc a.cpp",
			Sniff: `\(\d+(,\d+)?\): (fatal )?(error|warning) [CD]\d{4}:`, Sample: "a.cpp\na.cpp(12): error C2065: 'x': undeclared identifier\n"},
		Row{Type: "build/dmd", Cmds: []string{"dmd", "ldc2", "ldmd2", "gdc", "dub build"}, Cmd: "dub build",
			Sniff: `^\S+\.di?\(\d+(,\d+)?\): (Error|Warning|Deprecation):`, Sample: "source/app.d(12,5): Error: undefined identifier `x`\n"},
		Row{Type: "build/scala", Cmds: []string{"sbt", "scalac", "scala-cli", "mill", "scalafmt", "scalafix"}, Cmd: "sbt compile",
			Sniff: `^\[(error|warn)\] \S+\.scala:\d+:\d+: |^\[info\] (welcome to sbt|loading settings)`, Sample: "[info] welcome to sbt 1.9.7\n[error] /src/Main.scala:12:5: not found: value x\n"},
		Row{Type: "build/swift", Cmds: []string{"swift build", "swiftc", "xcodebuild"}, Cmd: "swift build",
			Sniff: `^\S+\.swift:\d+:\d+: (error|warning):`, Sample: "/src/main.swift:3:9: error: cannot find 'x' in scope\n"},
		Row{Type: "build/zig", Cmds: []string{"zig build", "zig build-exe", "zig build-lib"}, Cmd: "zig build",
			Sniff: `^\S+\.zig:\d+:\d+: error:`, Sample: "src/main.zig:5:9: error: use of undeclared identifier 'x'\n"},
		Row{Type: "build/kotlin", Cmds: []string{"kotlinc", "ktlint", "detekt"}, Cmd: "kotlinc Main.kt",
			Sniff: `^([ew]: )?(file://)?\S+\.kts?:\d+:\d+[: ]`, Sample: "e: file:///src/Main.kt:12:5 Unresolved reference: x\n"},
		Row{Type: "build/php", Cmds: []string{"phpstan", "psalm", "phpcs"}, Cmd: "phpstan analyse src"},
		Row{Type: "build/rubocop", Cmds: []string{"rubocop"}, Cmd: "rubocop",
			Sniff: `^\S+\.rb:\d+:\d+: [CWEFR]: `, Sample: "lib/a.rb:3:1: C: Style/FrozenStringLiteralComment: Missing frozen string literal comment.\n"},
		Row{Type: "build/pylint", Cmds: []string{"pylint"}, Cmd: "pylint src",
			Sniff: `^\S+\.py:\d+:\d+: [CRWEF]\d{4}: `, Sample: "a.py:1:0: C0114: Missing module docstring (missing-module-docstring)\n"},
		Row{Type: "build/lint", Cmds: []string{"biome", "oxlint", "stylelint", "markdownlint", "markdownlint-cli2", "shellcheck", "hadolint", "yamllint", "actionlint", "tflint"}, Cmd: "yamllint ."},
		Row{Type: "build/dart", Cmds: []string{"dart analyze", "flutter analyze"}, Cmd: "dart analyze",
			Sniff: `^\s+(error|warning|info) [-•] \S+\.dart:\d+:\d+ [-•] `, Sample: "  error - lib/main.dart:12:5 - Undefined name 'x' - undefined_identifier\n"},
		Row{Type: "build/elixir", Cmds: []string{"mix compile", "mix credo"}, Cmd: "mix compile"},
		Row{Type: "build/haskell", Cmds: []string{"ghc", "cabal build", "stack build"}, Cmd: "cabal build",
			Sniff: `^\S+\.l?hs:\d+:\d+: (error|warning)`, Sample: "src/Main.hs:12:5: error: [GHC-88464]\n    Variable not in scope: x\n"},
	)
}
