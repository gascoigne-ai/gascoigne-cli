package detect

// Task runners, monorepo tools, CI logs.
const NX = "task/nx"

func init() {
	register(
		Spec{Type: NX, Cmds: []string{"nx"}, Priority: 44, // before the tools it runs
			Sniff: anyLine(`^\s*> nx run |NX\s+(Successfully ran|Ran target|Running target)`)},
	)
}
