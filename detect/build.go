package detect

// Compilers, linters, type checkers, bundlers.
const (
	GoBuild = "build/go"
	TSC     = "build/tsc"
)

func init() {
	register(
		Spec{Type: GoBuild, Cmds: []string{"go build", "go vet", "go install"}, Priority: 51,
			Sniff: anyLine(`^# [\w./-]+$|^\S+\.go:\d+:\d+: `)},
		Spec{Type: TSC, Cmds: []string{"tsc"}, Priority: 52,
			Sniff: anyLine(`\.tsx?\(\d+,\d+\): error TS\d+|\.tsx?:\d+:\d+ - error TS\d+`)},
	)
}
