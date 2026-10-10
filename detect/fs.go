package detect

// Files and listings.
const (
	LS    = "fs/ls"
	Code  = "fs/code" // source files, language via Lang
	Tree  = "fs/tree"
	Paths = "fs/find" // path lists: find, fd, git ls-files, rg --files
	DU    = "fs/du"
	DF    = "fs/df"
)

func init() {
	register(
		Spec{Type: LS, Cmds: []string{"ls", "eza", "exa"}, Priority: 71,
			Sniff: all(anyLine(`^total \d+`), ratio(`^[-dlcbps][rwxsStT-]{9}[.+@]?\s`, 0.5))},
		Spec{Type: Tree, Cmds: []string{"tree", "eza --tree", "exa --tree", "lsd --tree"}, Priority: 70,
			Sniff: all(anyLine(`^(│   |    )*(├──|└──) `), ratio(`^(│   |    )*(├──|└──) |^\S+$|^\d+ director(y|ies)|^$`, 0.9))},
		Spec{Type: Paths, Cmds: []string{"find", "fd", "fdfind", "git ls-files", "rg --files", "locate", "plocate"}, Priority: 86,
			Sniff: ratio(`^(\./|/)?[\w.@+-]+(/[\w.@+ -]+)+/?$`, 0.95)},
		Spec{Type: DU, Cmds: []string{"du"}, Priority: 72, Sniff: ratio(`^\d+([.,]\d+)?[KMGTP]?\t\S`, 0.9)},
		Spec{Type: DF, Cmds: []string{"df"}, Priority: 72, Sniff: firstLine(`^Filesystem\s+(Type\s+)?(Size|1K-blocks|\d+[KM]?-blocks|Inodes)`)},
		Spec{Type: Code, Priority: 90, Sniff: func(t string, _ []string) bool { return Lang(t, Hints{}) != "" }},
	)
}
