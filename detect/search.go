package detect

// Search results.
const (
	Grep    = "search/grep" // grep, egrep, fgrep, ack
	RG      = "search/rg"
	AG      = "search/ag"
	GitGrep = "search/git-grep"
)

func init() {
	register(
		Spec{Type: Grep, Cmds: []string{"grep", "egrep", "fgrep", "ack", "zgrep"}, Priority: 70,
			Sniff: ratio(`^[^\s:\d][^\s:]*:\d+[:-]`, 0.6)}, // path can't start with a digit: timestamps
		Spec{Type: RG, Cmds: []string{"rg"}},
		Spec{Type: AG, Cmds: []string{"ag", "pt"}},
		Spec{Type: GitGrep, Cmds: []string{"git grep"}},
	)
}
