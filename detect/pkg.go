package detect

// Package managers.
const (
	NPM        = "pkg/npm"
	PNPM       = "pkg/pnpm"
	Yarn       = "pkg/yarn"
	BunInstall = "pkg/bun"
	Pip        = "pkg/pip"
	UV         = "pkg/uv"
	Poetry     = "pkg/poetry"
	GoMod      = "pkg/go-mod"
	CargoPkg   = "pkg/cargo"
	OSPkg      = "pkg/os" // apt, dnf, yum, brew, pacman, apk
)

func init() {
	register(
		// Only installs: npm test / run / exec run other tools, detected by sniffing.
		Spec{Type: NPM, Cmds: []string{"npm install", "npm i", "npm ci", "npm add", "npm update", "npm uninstall", "npm audit", "npm outdated"}, Priority: 57,
			Sniff: anyLine(`^npm (warn|WARN|ERR!|error|notice|http|info|verb|sill|timing) |^added \d+ packages?|^up to date, audited|^> \S+@\S+ \S+`)},
		Spec{Type: PNPM, Cmds: []string{"pnpm install", "pnpm i", "pnpm add", "pnpm update", "pnpm remove"}, Priority: 56,
			Sniff: anyLine(`^Progress: resolved \d+, reused \d+|^Packages: \+\d+`)},
		Spec{Type: Yarn, Cmds: []string{"yarn install", "yarn add", "yarn remove", "yarn upgrade"}, Priority: 56,
			CmdFunc: bare("yarn"), // yarn alone installs; yarn <script> runs other tools
			Sniff:   anyLine(`^\[\d/\d\] (Resolving|Fetching) packages|^➤ YN\d{4}: |^yarn install v\d`)},
		Spec{Type: BunInstall, Cmds: []string{"bun install", "bun add", "bun i", "bun remove", "bun update"}, Priority: 54,
			CmdFunc: bare("bun"),
			Sniff:   anyLine(`^bun (install|add|remove|update) v\d`)},
		Spec{Type: Pip, Cmds: []string{"pip install", "pip3 install", "pip uninstall"}, Priority: 57,
			Sniff: anyLine(`^Collecting \S+|^Requirement already satisfied: |^Successfully installed \S+`)},
		Spec{Type: UV, Cmds: []string{"uv pip", "uv sync", "uv add", "uv lock", "uv pip install"}, Priority: 56,
			Sniff: anyLine(`^(Resolved|Installed|Audited) \d+ packages? in [\d.]+m?s`)},
		Spec{Type: Poetry, Cmds: []string{"poetry install", "poetry add", "poetry update", "poetry lock"}, Priority: 56,
			Sniff: anyLine(`^Package operations: \d+ installs?|^Installing dependencies from lock file`)},
		Spec{Type: GoMod, Cmds: []string{"go mod", "go get"}, Priority: 56, Sniff: anyLine(`^go: (downloading|added|upgraded) \S+ v\d`)},
		Spec{Type: CargoPkg, Cmds: []string{"cargo add", "cargo update", "cargo fetch", "cargo install", "cargo remove"}, Priority: 57,
			Sniff: anyLine(`^\s+Adding \S+ v[\d.]+ to (dev-)?dependencies|^\s+Installed package `)},
		Spec{Type: OSPkg, Cmds: []string{"apt", "apt-get", "dnf", "yum", "brew", "pacman", "yay", "paru", "apk", "zypper"}, Priority: 57,
			Sniff: anyLine(`^(Get|Hit):\d+ https?://|^Reading package lists|^==> (Downloading|Pouring) |^:: Synchronizing package databases|^Last metadata expiration check|^\(\d+/\d+\) Installing `)},
	)
}

// bare matches prog with flags only: no subcommand or script name.
func bare(prog string) func(args []string) bool {
	return func(args []string) bool { return args[0] == prog && len(positional(args)) == 0 }
}
