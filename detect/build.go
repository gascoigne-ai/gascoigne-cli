package detect

// Compilers, linters, type checkers, bundlers.
const (
	GoBuild     = "build/go"
	TSC         = "build/tsc"
	ESLint      = "build/eslint"
	GolangCI    = "build/golangci"
	Ruff        = "build/ruff"
	Mypy        = "build/mypy"
	Rustc       = "build/rustc" // cargo build/check/clippy
	GCC         = "build/gcc"
	Clang       = "build/clang"
	Javac       = "build/javac"
	MavenBuild  = "build/maven"
	DotnetBuild = "build/dotnet"
	Bundler     = "build/bundler" // vite, webpack, rollup, esbuild
	DockerBuild = "build/docker"
)

func init() {
	register(
		Spec{Type: GoBuild, Cmds: []string{"go build", "go vet", "go install", "staticcheck"}, Priority: 51,
			Sniff: anyLine(`^# [a-z][\w./-]*$|^\S+\.go:\d+:\d+: `)}, // package paths are lowercase, headings rarely
		Spec{Type: TSC, Cmds: []string{"tsc", "vue-tsc"}, Priority: 52,
			Sniff: anyLine(`\.tsx?\(\d+,\d+\): error TS\d+|\.tsx?:\d+:\d+ - error TS\d+`)},
		Spec{Type: ESLint, Cmds: []string{"eslint", "next lint"}, Priority: 52,
			Sniff: anyLine(`^✖ \d+ problems? \(\d+ errors?, \d+ warnings?\)`)},
		Spec{Type: GolangCI, Cmds: []string{"golangci-lint"}, Priority: 49, Sniff: anyLine(`\.go:\d+:\d+: .* \((errcheck|govet|staticcheck|unused|gosimple|ineffassign|revive|gocritic)\)$`)},
		Spec{Type: Ruff, Cmds: []string{"ruff", "ruff check", "flake8"}, Priority: 52,
			Sniff: func(t string, l []string) bool {
				return ruffConcise(t, l) || ruffFull(t, l) && ruffLoc(t, l)
			}},
		Spec{Type: Mypy, Cmds: []string{"mypy", "pyright"}, Priority: 52, Sniff: anyLine(`^Found \d+ errors? in \d+ files? \(checked|^\S+\.py:\d+: error: .*\[[a-z-]+\]$`)},
		Spec{Type: Rustc, Cmds: []string{"cargo build", "cargo check", "cargo clippy", "rustc"}, Priority: 52,
			Sniff: all(anyLine(`^(error|warning)(\[E\d+\])?: `), anyLine(`^\s*--> \S+\.rs:\d+:\d+`))},
		Spec{Type: GCC, Cmds: []string{"gcc", "g++", "cc", "c++"}, Priority: 53, Sniff: anyLine(`^\S+\.(c|cc|cpp|cxx|h|hpp):\d+:\d+: (error|warning|fatal error): `)},
		Spec{Type: Clang, Cmds: []string{"clang", "clang++"}},
		Spec{Type: Javac, Cmds: []string{"javac"}, Priority: 53, Sniff: anyLine(`^\S+\.java:\d+: (error|warning): `)},
		Spec{Type: MavenBuild, Cmds: []string{"mvn compile", "mvn package", "mvn install", "mvnw compile", "mvnw package", "mvnw install"}, Priority: 53,
			Sniff: anyLine(`^\[ERROR\] (/\S+|\S+)\.java:\[\d+,\d+\] `)},
		Spec{Type: DotnetBuild, Cmds: []string{"dotnet build", "dotnet publish", "msbuild"}, Priority: 53,
			Sniff: anyLine(`\.(cs|fs|vb)proj\]$|: error (CS|FS|MSB|NU)\d+: `)},
		Spec{Type: Bundler, Cmds: []string{"vite", "vite build", "webpack", "rollup", "esbuild", "next build", "parcel"}, Priority: 53,
			Sniff: anyLine(`^vite v[\d.]+ building|^webpack \d[\d.]* compiled|^\[!\] (RollupError|Error)|^✘ \[ERROR\]|^\s*▲ Next\.js`)},
		Spec{Type: DockerBuild, Cmds: []string{"docker build", "docker buildx build", "podman build", "docker compose build"}, Priority: 45,
			Sniff: anyLine(`^#\d+ \[\s*\d+/\d+\] (FROM|RUN|COPY|ADD|WORKDIR)|^#\d+ \[internal\] load build definition`)},
	)
}

var (
	ruffConcise = anyLine(`^\S+\.py:\d+:\d+: [A-Z]+\d+ `)
	ruffFull    = anyLine(`^[A-Z]+\d+ (\[\*\] )?\S`)
	ruffLoc     = anyLine(`^\s*--> \S+\.py:\d+:\d+`)
)
