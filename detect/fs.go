package detect

// Files and listings.
const (
	LS   = "fs/ls"
	Code = "fs/code" // source files, language via Lang
)

func init() {
	register(
		Spec{Type: LS, Cmds: []string{"ls", "eza", "exa"}, Priority: 71,
			Sniff: all(anyLine(`^total \d+`), ratio(`^[-dlcbps][rwxsStT-]{9}[.+@]?\s`, 0.5))},
		Spec{Type: Code, Priority: 90, Sniff: func(t string, _ []string) bool { return Lang(t, Hints{}) != "" }},
	)
}
