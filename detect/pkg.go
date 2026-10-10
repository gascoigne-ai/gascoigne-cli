package detect

// Package managers.
const (
	NPM        = "pkg/npm" // npm, pnpm, yarn
	BunInstall = "pkg/bun"
)

func init() {
	register(
		Spec{Type: BunInstall, Cmds: []string{"bun", "bun install", "bun add", "bun i", "bun remove", "bun update"}, Priority: 54,
			Sniff: anyLine(`^bun (install|add|remove|update) v\d`)},
		Spec{Type: NPM, Cmds: []string{"npm", "pnpm", "yarn"}, Priority: 57,
			Sniff: anyLine(`^npm (warn|WARN|ERR!|error|notice|http|info|verb|sill|timing) |^added \d+ packages?|^up to date, audited|^> \S+@\S+ \S+`)},
	)
}
