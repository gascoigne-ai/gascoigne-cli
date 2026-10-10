package detect

// Task runners, monorepo tools, CI logs.
const (
	NX        = "task/nx"
	Turbo     = "task/turbo"
	Make      = "task/make"
	Just      = "task/just"
	Gradle    = "task/gradle"
	ComposeUp = "task/compose" // docker compose up
	CILog     = "task/ci"      // gh run view --log (GitHub Actions)
)

// Task runners sniff before the tools they run (their output contains them).
func init() {
	register(
		Spec{Type: NX, Cmds: []string{"nx"}, Priority: 44,
			Sniff: anyLine(`^\s*> nx run |NX\s+(Successfully ran|Ran target|Running target)`)},
		Spec{Type: Turbo, Cmds: []string{"turbo", "turbo run"}, Priority: 44,
			Sniff: anyLine(`^• (Packages in scope|Running \S+ in \d+ packages)|^\s*Tasks:\s+\d+ successful, \d+ total`)},
		Spec{Type: Make, Cmds: []string{"make", "gmake"}, Priority: 44,
			Sniff: anyLine(`^g?make(\[\d+\])?: (Entering directory|\*\*\* )`)},
		Spec{Type: Just, Cmds: []string{"just"}},
		Spec{Type: Gradle, Cmds: []string{"gradle", "gradlew"}, Priority: 44,
			Sniff: anyLine(`^> Task :\S+|^BUILD (SUCCESSFUL|FAILED) in \d`)},
		Spec{Type: ComposeUp, Cmds: []string{"docker compose up", "docker-compose up", "podman compose up", "docker compose logs"}, Priority: 44,
			Sniff: all(anyLine(`^(Attaching to |\s*\[\+\] Running \d+/\d+)`), ratio(`^\S+\s+\| `, 0.3))},
		Spec{Type: CILog, Cmds: []string{"gh run view --log", "gh run view --log-failed"}, Priority: 44,
			Sniff: ratio(`^[^\t]+\t[^\t]+\t\d{4}-\d\d-\d\dT[\d:.]+Z `, 0.5)},
	)
}
