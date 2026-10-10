package detect

// Task runners, monorepo tools, CI logs.
const NX = "task/nx"

func init() {
	register(
		Spec{Type: NX, Cmds: []string{"nx"}, Priority: 55,
			Sniff: anyLine(`^\s*> nx run |NX\s+(Successfully ran|Ran target|Running target)`)},
	)
}
