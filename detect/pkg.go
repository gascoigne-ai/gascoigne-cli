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
		// Only installs: npm test / run / exec run other tools, detected by sniffing.
		Spec{Type: NPM, Cmds: []string{
			"npm install", "npm i", "npm ci", "npm add", "npm update", "npm uninstall", "npm audit", "npm outdated",
			"pnpm install", "pnpm i", "pnpm add", "pnpm update", "pnpm remove", "pnpm audit",
			"yarn", "yarn install", "yarn add", "yarn remove", "yarn upgrade", "yarn audit",
		}, Priority: 57,
			Sniff: anyLine(`^npm (warn|WARN|ERR!|error|notice|http|info|verb|sill|timing) |^added \d+ packages?|^up to date, audited|^> \S+@\S+ \S+`)},
	)
}
