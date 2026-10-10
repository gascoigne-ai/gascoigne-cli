package detect

// Search results.
const Grep = "search/grep" // grep, rg, ag, ack, git grep

func init() {
	register(
		Spec{Type: Grep, Cmds: []string{"grep", "egrep", "fgrep", "rg", "ag", "ack", "git grep"}, Priority: 70,
			Sniff: ratio(`^[^\s:\d][^\s:]*:\d+[:-]`, 0.6)}, // path can't start with a digit: timestamps
	)
}
