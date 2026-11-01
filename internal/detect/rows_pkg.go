package detect

// Package managers declared as rows; rules per dialect are in compress/pkgnoise.
func init() {
	registerRows(
		Row{Type: "pkg/composer", Priority: 55, Cmds: []string{"composer install", "composer update", "composer require", "composer remove"}, Cmd: "composer install",
			Sniff: `^Loading composer repositories with package information|^Generating autoload files`, Sample: "Loading composer repositories with package information\nInstalling dependencies from lock file\n"},
		Row{Type: "pkg/gem", Cmds: []string{"gem install", "gem update", "bundle install", "bundle update", "bundle add"}, Cmd: "bundle install",
			Sniff: `^Fetching gem metadata from|^Bundle complete!`, Sample: "Fetching gem metadata from https://rubygems.org/.........\nResolving dependencies...\nBundle complete! 12 Gemfile dependencies, 60 gems now installed.\n"},
		Row{Type: "pkg/mix", Cmds: []string{"mix deps.get", "mix deps.update", "mix deps.compile"}, Cmd: "mix deps.get",
			Sniff: `^Resolving Hex dependencies|^Resolution completed in`, Sample: "Resolving Hex dependencies...\nResolution completed in 0.1s\nNew:\n  plug 1.15.3\n* Getting plug (Hex package)\n"},
		Row{Type: "pkg/pub", Cmds: []string{"dart pub", "flutter pub", "pub get"}, Cmd: "flutter pub get",
			Sniff: `^Got dependencies!|^Changed \d+ dependenc`, Sample: "Resolving dependencies...\n+ http 1.2.0\nGot dependencies!\n"},
		Row{Type: "pkg/swift", Cmds: []string{"swift package"}, Cmd: "swift package resolve",
			Sniff: `^Fetching https://|^Computing version for |^Creating working copy for `, Sample: "Fetching https://github.com/apple/swift-argument-parser from cache\nComputing version for https://github.com/apple/swift-argument-parser\n"},
		Row{Type: "pkg/nuget", Cmds: []string{"dotnet restore", "dotnet add", "dotnet remove", "dotnet list", "nuget"}, Cmd: "dotnet restore",
			Sniff: `^\s+Restored \S+\.csproj|^\s+(GET|OK) https://api\.nuget\.org`, Sample: "  Determining projects to restore...\n  Restored /src/App/App.csproj (in 1.2 sec).\n"},
		Row{Type: "pkg/conda", Priority: 55, Cmds: []string{"conda", "mamba", "micromamba"}, Cmd: "conda install numpy",
			Sniff: `^Collecting package metadata|^Solving environment: |^## Package Plan ##|^Preparing transaction: `, Sample: "Collecting package metadata (current_repodata.json): done\nSolving environment: done\n"},
		Row{Type: "pkg/deno", Cmds: []string{"deno install", "deno add"}, Cmd: "deno install",
			Sniff: `^Download https://(jsr\.io|deno\.land|registry\.npmjs\.org)`, Sample: "Download https://jsr.io/@std/path/meta.json\n"},
		Row{Type: "pkg/nix", Priority: 40, Cmds: []string{"nix build", "nix develop", "nix-build", "nix-env", "nix-shell", "nix profile"}, Cmd: "nix build .#shop",
			Sniff: `^(copying path '/nix/store|building '/nix/store/\S+\.drv'|these \d+ (derivations|paths) will be (built|fetched))`, Sample: "these 3 derivations will be built:\n  /nix/store/abc-shop.drv\nbuilding '/nix/store/abc-shop.drv'...\n"},
		Row{Type: "pkg/flatpak", Cmds: []string{"flatpak install", "flatpak update", "snap install", "snap refresh"}, Cmd: "flatpak install flathub org.gimp.GIMP"},
		Row{Type: "pkg/mvndeps", Cmds: []string{"mvn dependency:tree", "mvn dependency:resolve", "mvn dependency:go-offline", "mvnw dependency:tree"}, Cmd: "mvn dependency:tree"},
		Row{Type: "pkg/docker", Priority: 55, Cmds: []string{"docker pull", "docker push", "docker image pull", "docker image push", "docker compose pull", "docker-compose pull", "podman pull", "podman push"}, Cmd: "docker pull alpine",
			Sniff: `^[0-9a-f]{12}: (Pulling fs layer|Preparing)|^The push refers to repository `, Sample: "latest: Pulling from library/alpine\n3f4ca61aafcd: Pulling fs layer\n3f4ca61aafcd: Pull complete\nDigest: sha256:3f4ca61aafcd\n"},
		// Inventories, one package per line; pacman -Q, brew list --versions and go list -m all have no sniffable header.
		Row{Type: "pkg/list", Cmds: []string{"pip list", "pip3 list", "pip freeze", "pip3 freeze", "uv pip list", "uv pip freeze", "apt list", "pacman -Q", "pacman -Qe", "pacman -Qm", "yay -Q", "paru -Q",
			"brew list", "gem list"}, Cmd: "pip list", // go list -m: CmdFunc in pkg.go
			Sniff: `^Package\s+Version(\s|$)|^Listing\.\.\.( Done)?$|^\*\*\* LOCAL GEMS \*\*\*`, Sample: "Package    Version\n---------- -------\nflask      3.0.3\nrequests   2.32.3\n"},
		// Glyph trees; the engine reads any of them, so only tools with a real fixture are listed.
		Row{Type: "pkg/tree", Priority: 55, Cmds: []string{"npm ls", "npm list", "npm ll", "pnpm ls", "pnpm list", "pnpm why", "pipdeptree", "uv tree", "poetry show --tree"}, Cmd: "npm ls --all"},
		Row{Type: "pkg/gomodgraph", Priority: 55, Cmds: []string{"go mod graph"}, Cmd: "go mod graph"},
	)
}
