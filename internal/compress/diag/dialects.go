package diag

import (
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

func re(s string) *lazyre.Regexp { return lazyre.New(s) }

var mavenNoise = pcomb.Rules{
	{
		Name:  "downloads",
		Match: pcomb.Any(pcomb.Prefix("Downloading from ", "Downloaded from ", "Progress ("), pcomb.Prefixed("[INFO] ", pcomb.Prefix("Download"))),
		Yes:   []string{"Downloading from central: https://repo.maven.apache.org/maven2/a/a.pom", "Downloaded from central: https://repo.maven.apache.org/maven2/a/a.pom (4.1 kB at 12 kB/s)", "Progress (1): 4.1/12 kB", "[INFO] Downloading from central: https://repo.maven.apache.org/maven2/a/a.jar"},
		No:    []string{"[ERROR] Downloading failed", "[INFO] Building app 1.0"},
	},
	{
		Name:  "separators",
		Match: pcomb.Prefixed("[INFO] ", pcomb.Any(pcomb.Prefix("---"), pcomb.Re(`^\s*$`), pcomb.Re(`^-+$`))),
		Yes:   []string{"[INFO] --- maven-compiler-plugin:3.11.0:compile (default-compile) @ app ---", "[INFO] ", "[INFO] ------------------------------------------------------------------------"},
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
	{
		Name:  "module header",
		Match: pcomb.Prefixed("[INFO] ", pcomb.Re(`^Building \S+ \S+$`)),
		Yes:   []string{"[INFO] Building app 1.0-SNAPSHOT", "[INFO] Building jar: /src/target/app.jar"},
		No:    []string{"[INFO] Building app", "[ERROR] Building app 1.0"},
	},
}

// rules per build dialect; unknown dialects get no noise rules.
var rules = map[string]Rules{
	"rustc": {Noise: pcomb.Rules{{
		Name:  "cargo progress",
		Match: pcomb.Indented(pcomb.Word("Compiling", "Checking", "Downloaded", "Downloading", "Updating", "Locking", "Blocking", "Fresh", "Finished")),
		Yes:   []string{"   Compiling serde v1.0.210", "    Finished `dev` profile [unoptimized] target(s) in 2.31s", "    Blocking waiting for file lock on build directory"},
		No:    []string{"Compiling serde v1.0.210", "error[E0308]: mismatched types", "   Compilingly"},
	}}},
	"javac": {Noise: mavenNoise},
	"maven": {Noise: mavenNoise},
	"dotnet": {Noise: pcomb.Rules{
		{
			Name:  "restore chatter",
			Match: pcomb.Trimmed(pcomb.Prefix("Determining projects to restore", "All projects are up-to-date", "Restored ", "MSBuild version", "Time Elapsed", "Workload updates")),
			Yes:   []string{"  Determining projects to restore...", "  Restored /src/App/App.csproj (in 312 ms).", "Time Elapsed 00:00:03.21", "MSBuild version 17.8.3+195e7f5a3 for .NET"},
			No:    []string{"Program.cs(4,5): error CS1002: ; expected", "  Restoring"},
		},
		{
			Name:  "output path",
			Match: pcomb.Re(`^\s*\S+ -> /`),
			Yes:   []string{"  App -> /src/App/bin/Debug/net8.0/App.dll"},
			No:    []string{"App -> bin/Debug/App.dll"},
		},
		{
			Name:  "warning and error counts",
			Match: pcomb.Re(`^\s*\d+ (Warning|Error)\(s\)$`),
			Yes:   []string{"    0 Warning(s)", "    2 Error(s)"},
			No:    []string{"    2 Error(s) found", "Warning(s)"},
		},
	}},
	"bundler": {Noise: pcomb.Rules{
		{
			Name:  "vite progress",
			Match: pcomb.Any(pcomb.Re(`^vite v[\d.]+ building`), pcomb.Prefix("transforming...", "rendering chunks...", "computing gzip size...")),
			Yes:   []string{"vite v5.2.0 building for production...", "transforming...", "rendering chunks...", "computing gzip size..."},
			No:    []string{"vite building", "error during build:"},
		},
		{
			Name:  "vite module count",
			Match: pcomb.Re(`✓ \d+ modules transformed`),
			Yes:   []string{"✓ 143 modules transformed."},
			No:    []string{"✓ modules transformed"},
		},
		{
			Name:  "vite asset table",
			Match: pcomb.Re(`^\S+\s+[\d.]+ kB\s*│`),
			Yes:   []string{"dist/assets/index-4d5f.js   143.22 kB │ gzip: 46.10 kB"},
			No:    []string{"dist/index.html 0.5 kB"},
		},
		{
			Name:  "webpack assets",
			Match: pcomb.Any(pcomb.Re(`^asset \S+ [\d.]+ \w?i?B`), pcomb.Prefix("orphan modules", "runtime modules", "cacheable modules"), pcomb.Indented(pcomb.Prefix("modules by path")), pcomb.Re(`^\s+\./\S+ [\d.]+ \w?i?B`)),
			Yes:   []string{"asset main.js 1.2 KiB [emitted] (name: main)", "orphan modules 12 KiB [orphan] 3 modules", "runtime modules 663 bytes 3 modules", "cacheable modules 1.5 KiB", "  modules by path ./node_modules/ 1.1 KiB", "  ./src/index.js 1.1 KiB [built] [code generated]"},
			No:    []string{"asset main.js", "ERROR in ./src/index.js"},
		},
		{
			Name:  "webpack module flags",
			Match: pcomb.Trimmed(pcomb.Prefix("[built]", "[code generated]")),
			Yes:   []string{"[built]", "  [code generated]"},
			No:    []string{"[failed]"},
		},
		{
			Name:  "node_modules frames",
			Match: pcomb.Re(`^\s+at .*node_modules`),
			Yes:   []string{"    at Object.<anonymous> (/src/node_modules/webpack/lib/x.js:10:5)"},
			No:    []string{"    at main (/src/app.js:3:1)", "at x node_modules"},
		},
	}},
	"docker": {Strip: re(`^#\d+ \d+\.\d+( |$)|^\d+\.\d+ `), Noise: pcomb.Rules{
		{
			Name:  "buildkit step status",
			Match: pcomb.Re(`^#\d+ (\[internal\]|DONE|CACHED)`),
			Yes:   []string{"#5 [internal] load build definition from Dockerfile", "#5 DONE 0.0s", "#6 CACHED"},
			No:    []string{"#5 ERROR: process failed", "5 DONE"},
		},
		{
			Name:  "buildkit context and export",
			Match: pcomb.Re(`^#\d+ (resolve |transferring |naming |exporting |writing |sha256:|load build definition|load \.dockerignore|load metadata)`),
			Yes:   []string{"#3 resolve docker.io/library/alpine:3.19@sha256:abc", "#4 transferring context: 2B done", "#9 naming to docker.io/app:latest", "#9 exporting to image", "#9 writing image sha256:abc done", "#2 load metadata for docker.io/library/alpine:3.19", "#2 load .dockerignore"},
			No:    []string{"#7 RUN npm install", "#7 load something"},
		},
	}},
	"gcc":    {MaxFrame: 3},
	"clang":  {MaxFrame: 3},
	"go":     {},
	"tsc":    {},
	"eslint": {},
	"ruff":   {},
	"mypy":   {},
	"golangci": {Noise: pcomb.Rules{{
		Name:  "log level",
		Match: pcomb.Prefix("level=info", "level=debug"),
		Yes:   []string{`level=info msg="[config_reader] Used config file .golangci.yml"`, `level=debug msg="Linters: [errcheck govet]"`},
		No:    []string{`level=error msg="Running error: context loading failed"`, `main.go:4:2: unused (U1000)`},
	}}},

	"cmake": {Noise: pcomb.Rules{
		{
			Name:  "configure probes",
			Match: pcomb.Prefixed("-- ", pcomb.Any(pcomb.Re(`^The (C|CXX|ASM|CUDA) compiler`), pcomb.Prefix("Detecting", "Check for", "Looking for", "Performing Test", "Found ", "Configuring done", "Generating done", "Build files have been written", "Using "))),
			Yes:   []string{"-- The C compiler identification is GNU 13.2.1", "-- Detecting CXX compile features - done", "-- Check for working C compiler: /usr/bin/cc - works", "-- Looking for pthread.h", "-- Performing Test HAVE_X - Success", "-- Found Threads: TRUE", "-- Configuring done (0.3s)", "-- Generating done (0.0s)", "-- Build files have been written to: /src/build", "-- Using Python 3.12"},
			No:    []string{"-- Configuring incomplete, errors occurred!", "CMake Error at CMakeLists.txt:4"},
		},
		{
			Name:  "build progress",
			Match: pcomb.Any(pcomb.Re(`^\[\s*\d+%\] (Building|Linking|Built target|Generating|Scanning|Automatic)`), pcomb.Prefix("Scanning dependencies of target"), pcomb.Re(`^g?make\[\d+\]: (Entering|Leaving)`)),
			Yes:   []string{"[ 50%] Building CXX object CMakeFiles/app.dir/main.cpp.o", "[100%] Linking CXX executable app", "[100%] Built target app", "Scanning dependencies of target app", "make[2]: Entering directory '/src/build'", "gmake[1]: Leaving directory '/src/build'"},
			No:    []string{"make[2]: *** [Makefile:4: all] Error 1", "[ 50%] Error"},
		},
	}},
	"ninja": {Noise: pcomb.Rules{{
		Name:  "build step",
		Match: pcomb.Re(`^\[\d+/\d+\] (Building|Linking|Generating|Running|Install|Re-running|Creating|Scanning)`),
		Yes:   []string{"[3/12] Building CXX object src/CMakeFiles/app.dir/main.cpp.o", "[12/12] Linking CXX executable app"},
		No:    []string{"FAILED: src/CMakeFiles/app.dir/main.cpp.o", "[3/12] Failed"},
	}}},
	"meson": {Noise: pcomb.Rules{
		{
			Name:  "setup probes",
			Match: pcomb.Word("Run-time dependency", "Library", "Program", "Dependency", "Compiler for", "Build targets in project", "Found ninja", "Project name", "Project version", "Host machine", "Build machine", "Has header", "Checking for", "Configuring"),
			Yes:   []string{"Run-time dependency zlib found: YES 1.3", "Library m found: YES", "Program python3 found: YES (/usr/bin/python3)", "Compiler for C supports arguments -Wall: YES", "Project name: app", "Host machine cpu family: x86_64", "Checking for function \"strlcpy\" : NO", "Configuring config.h using configuration"},
			No:    []string{"meson.build:4:2: ERROR: Dependency \"x\" not found", "Libraryx"},
		},
		{
			Name:  "meson message",
			Match: pcomb.Re(`^Message:\b`),
			Yes:   []string{"Message:hello"},
			No:    []string{"Message: hello", "Messages"},
		},
		{
			Name:  "build step",
			Match: pcomb.Re(`^\[\d+/\d+\] (Compiling|Linking|Generating)`),
			Yes:   []string{"[1/9] Compiling C object app.p/main.c.o", "[9/9] Linking target app"},
			No:    []string{"[1/9] Failed"},
		},
	}},
	"bazel": {Noise: pcomb.Rules{
		{
			Name:  "progress",
			Match: pcomb.Re(`^(\(\d\d:\d\d:\d\d\) )?(Loading:|Analyzing:|DEBUG:|\[[\d,]+ / [\d,]+\]|Computing main repo mapping)`),
			Yes:   []string{"Loading: 0 packages loaded", "(10:04:05) Analyzing: target //app:app (1 packages loaded)", "[12 / 40] Compiling app/main.cc", "(10:04:06) [1,203 / 1,400] 3 actions running", "DEBUG: /src/x.bzl:3:1: note", "Computing main repo mapping:"},
			No:    []string{"ERROR: /src/app/BUILD:3:1: error", "(10:04:05) ERROR: x"},
		},
		{
			Name:  "info lines",
			Match: pcomb.Re(`^(\(\d\d:\d\d:\d\d\) )?INFO: (Analyzed|Found \d|From |Build options|Invocation|Elapsed|Streaming|Current date|Reading|Repository|Writing)`),
			Yes:   []string{"INFO: Analyzed target //app:app (0 packages loaded).", "(10:04:07) INFO: Found 1 target...", "INFO: From Compiling app/main.cc:", "INFO: Elapsed time: 3.2s", "INFO: Invocation ID: abc"},
			No:    []string{"INFO: Build completed successfully, 5 total actions", "INFO: Analyzing"},
		},
		{
			Name:  "fetching",
			Match: pcomb.Trimmed(pcomb.Prefix("Fetching")),
			Yes:   []string{"Fetching https://mirror/x.tar.gz", "  Fetching repository @zlib"},
			No:    []string{"Fetch failed"},
		},
	}},
	"cpplint": {Noise: pcomb.Rules{
		{
			Name:  "clang-tidy suppression notes",
			Match: pcomb.Any(pcomb.Re(`^\d+ warnings? generated\.`), pcomb.Re(`^Suppressed \d+ warnings`), pcomb.Prefix("Use -header-filter", "Use -system-headers")),
			Yes:   []string{"3 warnings generated.", "1 warning generated.", "Suppressed 3 warnings (3 in non-user code).", "Use -header-filter=.* to display errors from all non-system headers.", "Use -system-headers to display errors from system headers as well."},
			No:    []string{"warnings generated", "src/a.cc:4:1: warning: x [readability-x]"},
		},
		{
			Name:  "cpplint progress",
			Match: pcomb.Any(pcomb.Re(`^Checking \S+`), pcomb.Re(`^\d+/\d+ files checked`)),
			Yes:   []string{"Checking src/a.cc", "3/10 files checked"},
			No:    []string{"Checking", "files checked"},
		},
	}, MaxFrame: 3},
	"msvc": {Noise: pcomb.Rules{
		{
			Name:  "banner",
			Match: pcomb.Prefix("Microsoft (R)", "Copyright (C) Microsoft"),
			Yes:   []string{"Microsoft (R) C/C++ Optimizing Compiler Version 19.38", "Copyright (C) Microsoft Corporation.  All rights reserved."},
			No:    []string{"Microsoft Windows", "Copyright (C) 2024"},
		},
		{
			Name:  "compiled file name",
			Match: pcomb.Re(`^\S+\.(cpp|cc|c|cxx)\s*$`),
			Yes:   []string{"main.cpp", "util.c   "},
			No:    []string{"main.cpp(4): error C2143", "main.h"},
		},
		{
			Name:  "link steps",
			Match: pcomb.Any(pcomb.Prefix("Generating Code", "Finished Generating"), pcomb.Indented(pcomb.Prefix("Creating library"))),
			Yes:   []string{"Generating Code...", "Finished Generating Code", "   Creating library app.lib and object app.exp"},
			No:    []string{"Generating", "Creating library app.lib"},
		},
	}},
	"dmd": {Noise: pcomb.Rules{{
		Name:  "dub progress",
		Match: pcomb.Any(pcomb.Prefix("Performing \"", "Starting Performing", "Running ldc2", "Running dmd", "Linking", "Compiling", "Fetching", "Placing", "Using ", "Selecting", "Basic dub"), pcomb.Re(`^Building \S+ \S+: building configuration`), pcomb.Trimmed(pcomb.Prefix("Up-to-date"))),
		Yes:   []string{"Performing \"debug\" build using dmd for x86_64.", "Starting Performing \"debug\" build", "Building app ~master: building configuration [application]", "Running dmd...", "Linking...", "Compiling...", "  Up-to-date vibe-d 0.9.5", "Fetching vibe-d 0.9.5", "Using dub.json", "Selecting vibe-d v0.9.5", "Placing vibe-d"},
		No:    []string{"source/app.d(4,5): Error: undefined identifier x", "Performing debug build"},
	}}},
	"scala": {
		Tag: re(`^\[(error|warn)\] `),
		Noise: pcomb.Rules{
			{
				Name:  "sbt info",
				Match: pcomb.Prefixed("[info] ", pcomb.Any(pcomb.Prefix("welcome to sbt", "loading ", "set current project", "done compiling", "Updating", "Resolving", "Fetched", "resolving key", "Non-compiled module", "Compilation completed"), pcomb.Re(`^compiling \d+ `))),
				Yes:   []string{"[info] welcome to sbt 1.9.7", "[info] loading project definition from /src/project", "[info] set current project to app", "[info] compiling 3 Scala sources to /src/target", "[info] done compiling", "[info] Updating", "[info] Resolving org.scala-lang#scala-library;2.13.12 ...", "[info] Fetched artifacts of", "[info] resolving key references (12301 settings)", "[info] Non-compiled module 'compiler-bridge'", "[info] Compilation completed in 3.1s."},
				No:    []string{"[error] compiling failed", "[warn] loading x"},
			},
			{
				Name:  "sbt success",
				Match: pcomb.Prefix("[success] "),
				Yes:   []string{"[success] Total time: 4 s, completed May 1, 2024, 10:00:00 AM"},
				No:    []string{"[success]", "success"},
			},
		},
		MaxFrame: 6,
	},
	"swift": {Noise: pcomb.Rules{
		{
			Name:  "swiftpm build steps",
			Match: pcomb.Any(pcomb.Re(`^\[\d+/\d+\] (Compiling|Emitting|Write|Linking)`), pcomb.Prefix("Building for ", "Build complete", "Compiling ")),
			Yes:   []string{"[3/9] Compiling App main.swift", "[4/9] Emitting module App", "[8/9] Write Objects.LinkFileList", "[9/9] Linking app", "Building for debugging...", "Build complete! (2.31s)", "Compiling App main.swift"},
			No:    []string{"main.swift:4:5: error: cannot find 'x' in scope", "[3/9] Failed"},
		},
		{
			Name:  "swiftpm dependencies",
			Match: pcomb.Prefix("Fetching ", "Computing version", "Creating working", "Resolving", "Fetched", "Cloning"),
			Yes:   []string{"Fetching https://github.com/apple/swift-argument-parser from cache", "Computing version for https://github.com/x/y", "Creating working copy of package 'y'", "Resolving https://github.com/x/y", "Fetched https://github.com/x/y (0.5s)", "Cloning https://github.com/x/y"},
			No:    []string{"error: Fetching failed", "fetching x"},
		},
	}},
	"zig": {Noise: pcomb.Rules{{
		Name:  "build step tree",
		Match: pcomb.Trimmed(pcomb.Prefix("+- ")),
		Yes:   []string{"+- compile exe app Debug native 1 errors", "   +- compile exe app Debug native"},
		No:    []string{"src/main.zig:4:5: error: expected ';'", "+-"},
	}}},
	"kotlin": {},
	"php":    {},
	"rubocop": {Noise: pcomb.Rules{
		{
			Name:  "inspecting banner",
			Match: pcomb.Re(`^Inspecting \d+ files?`),
			Yes:   []string{"Inspecting 12 files", "Inspecting 1 file"},
			No:    []string{"Inspecting files"},
		},
		{
			Name:  "progress dots",
			Match: pcomb.Re(`^[.CWEFR]+$`),
			Yes:   []string{"..C.W..E", "R"},
			No:    []string{"", "..x", "app/models/user.rb:3:1: C: Style/Foo"},
		},
		{
			Name:  "offenses header",
			Match: pcomb.Re(`^Offenses:$`),
			Yes:   []string{"Offenses:"},
			No:    []string{"Offenses: 3", "12 files inspected, 3 offenses detected"},
		},
	}},
	"pylint": {Noise: pcomb.Rules{{
		Name:  "module banner",
		Match: pcomb.Re(`^\*{5,} Module `),
		Yes:   []string{"************* Module app.models"},
		No:    []string{"app/models.py:3:0: C0114: Missing module docstring (missing-module-docstring)", "**** Module x"},
	}}},
	"lint": {},
	"dart": {Noise: pcomb.Rules{{
		Name:  "analyzing banner",
		Match: pcomb.Re(`^Analyzing \S+\.\.\.`),
		Yes:   []string{"Analyzing my_app...", "Analyzing lib..."},
		No:    []string{"  error - lib/main.dart:3:5 - Undefined name 'x'", "Analyzing"},
	}}},
	"elixir": {Noise: pcomb.Rules{
		{
			Name:  "mix steps",
			Match: pcomb.Any(pcomb.Prefix("==> "), pcomb.Re(`^Compiling \d+ files? \(\.ex\)`), pcomb.Re(`^Generated \S+ app`)),
			Yes:   []string{"==> app", "Compiling 4 files (.ex)", "Compiling 1 file (.ex)", "Generated app app"},
			No:    []string{"** (CompileError) lib/app.ex:4: undefined function x/0", "Compiling lib/app.ex"},
		},
	}},
	"haskell": {Noise: pcomb.Rules{
		{
			Name:  "ghc progress",
			Match: pcomb.Any(pcomb.Re(`^\[\s*\d+ of \d+\] Compiling`), pcomb.Prefix("Linking ", "Loaded package", "Preprocessing ", "Building ", "Configuring ")),
			Yes:   []string{"[1 of 3] Compiling Main ( app/Main.hs, dist/Main.o )", "[ 2 of 12] Compiling Lib", "Linking dist/app ...", "Loaded package environment from /home/u/.ghc/env", "Preprocessing executable 'app' for app-0.1.0.0..", "Building executable 'app' for app-0.1.0.0..", "Configuring app-0.1.0.0..."},
			No:    []string{"app/Main.hs:4:5: error:", "Linking"},
		},
		{
			Name:  "cabal plan",
			Match: pcomb.Prefix("In order, the following", "Resolving dependencies", "Build profile", "Up to date", "Downloading", "Starting", "Completed"),
			Yes:   []string{"In order, the following will be built (use -v for more details):", "Resolving dependencies...", "Build profile: -w ghc-9.6.4 -O1", "Up to date", "Downloading text-2.1", "Starting     text-2.1 (lib)", "Completed    text-2.1 (lib)"},
			No:    []string{"cabal: Could not resolve dependencies:", "Start"},
		},
	}, MaxFrame: 10},
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
