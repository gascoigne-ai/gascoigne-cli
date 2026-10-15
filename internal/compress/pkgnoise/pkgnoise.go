// Package pkgnoise compresses package manager output: progress, downloads,
// cache hits and boilerplate go; repeated warnings fold into one line per
// kind; summaries, added/removed packages, vulnerabilities and errors stay.
package pkgnoise

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

// Rules describe a manager.
type Rules struct {
	Noise pcomb.Rules // dropped
	// Fold groups lines into "kind (N): items"; group 1 is the item.
	Fold []Fold
}

type Fold struct {
	Kind string
	Re   *lazyre.Regexp
}

func re(s string) *lazyre.Regexp { return lazyre.New(s) }

var rules = map[string]Rules{
	"npm": {
		Noise: pcomb.Rules{
			{
				Name:  "log levels",
				Match: pcomb.Prefixed("npm ", pcomb.Word("http", "sill", "silly", "verb", "verbose", "timing", "notice", "info")),
				Yes:   []string{"npm http fetch GET 200 https://registry.npmjs.org/x 120ms", "npm notice New major version available", "npm timing idealTree Completed in 12ms"},
				No:    []string{"npm warn deprecated x@1: gone", "npm httpx"},
			},
			{
				Name:  "funding",
				Match: pcomb.Any(pcomb.Re(`^\d+ packages? (are|is) looking for funding`), pcomb.Trimmed(pcomb.Prefix("run `npm fund` for details"))),
				Yes:   []string{"12 packages are looking for funding", "1 package is looking for funding", "  run `npm fund` for details"},
				No:    []string{"funding", "npm fund"},
			},
			{
				Name:  "audit advice",
				Match: pcomb.Any(pcomb.Re(`^To address (all )?issues`), pcomb.Trimmed(pcomb.Prefix("npm audit fix")), pcomb.Prefix("Some issues need review", "a different dependency", "Run `npm audit` for details")),
				Yes:   []string{"To address all issues (including breaking changes), run:", "To address issues that do not require attention, run:", "  npm audit fix --force", "Some issues need review, and may require choosing", "a different dependency.", "Run `npm audit` for details."},
				No:    []string{"npm ERR! audit", "To fix"},
			},
		},
		Fold: []Fold{{"npm warn deprecated", re(`^npm (?:warn|WARN) deprecated (\S+?):`)}},
	},
	"pnpm": {
		Noise: pcomb.Rules{
			{
				Name:  "progress",
				Match: pcomb.Prefix("Progress: resolved", "Downloading "),
				Yes:   []string{"Progress: resolved 120, reused 100, downloaded 20, added 0", "Downloading registry.npmjs.org/x/1.0.0: 1.2 MB"},
				No:    []string{"Progress resolved", " Progress: resolved 1"},
			},
			{
				Name:  "summary",
				Match: pcomb.Re(`^Done in [\d.]+m?s`),
				Yes:   []string{"Done in 3.2s", "Done in 450ms"},
				No:    []string{"Done in a while"},
			},
			{
				Name:  "separators",
				Match: pcomb.Re(`^[+-]{5,}$`),
				Yes:   []string{"++++++++++------", "-----"},
				No:    []string{"++++", "+++++ x"},
			},
			{
				Name:  "box drawing",
				Match: pcomb.Any(pcomb.Trimmed(pcomb.Prefix("╭", "╰")), pcomb.Re(`^\s*│\s*$`)),
				Yes:   []string{"╭──────────╮", "  ╰──────────╯", "│", "  │  "},
				No:    []string{"│ Update available │", "x ╭"},
			},
		},
		Fold: []Fold{{"WARN deprecated", re(`^\s*WARN\s+deprecated (\S+?):`)}, {"WARN deprecated subdependencies", re(`^\s*WARN\s+\d+ deprecated subdependencies found: (.+)$`)}},
	},
	"yarn": {
		Noise: pcomb.Rules{
			{
				Name:  "v1 steps",
				Match: pcomb.Re(`^\[\d/\d\] (Resolving|Fetching|Linking|Building)`),
				Yes:   []string{"[1/4] Resolving packages...", "[3/4] Linking dependencies..."},
				No:    []string{"[1/4] Validating", "[10/40] Resolving"},
			},
			{
				Name:  "v1 info",
				Match: pcomb.Any(pcomb.Prefixed("info ", pcomb.Prefix("No lockfile", "Visit", "There appears", "fsevents")), pcomb.Prefix("success Saved lockfile", "yarn install v"), pcomb.Re(`^Done in [\d.]+s`)),
				Yes:   []string{"info No lockfile found.", "info Visit https://yarnpkg.com/en/docs/cli/install for documentation about this command.", "info fsevents@2.3.2: The platform \"linux\" is incompatible", "success Saved lockfile.", "yarn install v1.22.19", "Done in 3.21s"},
				No:    []string{"error An unexpected error occurred", "info Something else", "Done in 3m"},
			},
			{
				Name:  "berry",
				Match: pcomb.Any(pcomb.Re(`^➤ YN0000: [┌└│]`), pcomb.Prefix("➤ YN0013")),
				Yes:   []string{"➤ YN0000: ┌ Resolution step", "➤ YN0000: └ Completed", "➤ YN0013: │ x@npm:1.0.0 can't be found in the cache"},
				No:    []string{"➤ YN0000: Done in 1s", "➤ YN0001: Exception"},
			},
		},
		Fold: []Fold{{"warning deprecated", re(`^warning (\S+?): .*deprecated`)}, {"unmet peer dependency", re(`^warning "(\S+?)" has (?:unmet|incorrect) peer dependency`)}},
	},
	"bun": {
		Noise: pcomb.Rules{
			{
				Name:  "timings and status",
				Match: pcomb.Any(pcomb.Re(`^\[\d+(\.\d+)?m?s\] `), pcomb.Prefix("Extracted ", "Resolving dependencies", "Resolved, downloaded and extracted", "Saved lockfile")),
				Yes:   []string{"[12.00ms] Resolving", "[1.2s] Installed", "[5ms] done", "Extracted 12 packages", "Resolving dependencies", "Resolved, downloaded and extracted [34]", "Saved lockfile"},
				No:    []string{"[x] y", "error: missing package"},
			},
		},
	},
	"pip": {
		Noise: pcomb.Rules{
			{
				Name:  "top level progress",
				Match: pcomb.Any(pcomb.Prefix("Collecting ", "Requirement already satisfied", "Installing collected packages", "Obtaining ", "Downloading ", "Using cached "), pcomb.Re(`^Building wheels? for`)),
				Yes:   []string{"Collecting requests", "Requirement already satisfied: idna in /usr/lib/python3 (3.4)", "Installing collected packages: idna, requests", "Obtaining file:///src", "Downloading requests-2.31.0-py3-none-any.whl (62 kB)", "Using cached requests-2.31.0.whl", "Building wheel for foo (pyproject.toml) ... done", "Building wheels for collected packages: foo"},
				No:    []string{"ERROR: Could not find a version", "Collectingx"},
			},
			{
				Name:  "indented progress",
				Match: pcomb.Indented(pcomb.Prefix("Downloading ", "Using cached ", "━", "Attempting uninstall", "Found existing installation", "Uninstalling ", "Successfully uninstalled", "Building wheel for", "Created wheel for", "Stored in directory", "Installing build dependencies", "Getting requirements to build")),
				Yes:   []string{"  Downloading urllib3-2.0.7-py3-none-any.whl (124 kB)", "     ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ 62.6/62.6 kB 1.2 MB/s eta 0:00:00", "  Attempting uninstall: idna", "    Found existing installation: idna 3.3", "    Uninstalling idna-3.3:", "      Successfully uninstalled idna-3.3", "  Building wheel for foo (pyproject.toml) ... done", "  Created wheel for foo: filename=foo.whl size=1 sha256=abc", "  Stored in directory: /home/u/.cache/pip/wheels/ab", "  Installing build dependencies ... done", "  Getting requirements to build wheel ... done"},
				No:    []string{"Downloading x", "  ERROR: bad"},
			},
			{
				Name:  "metadata",
				Match: pcomb.Trimmed(pcomb.Prefix("Preparing metadata")),
				Yes:   []string{"  Preparing metadata (pyproject.toml) ... done", "Preparing metadata (setup.py) ... done"},
				No:    []string{"Preparing wheel"},
			},
		},
	},
	"uv": {
		Noise: pcomb.Rules{
			{
				Name:  "progress",
				Match: pcomb.Trimmed(pcomb.Word("Downloading", "Building", "Built", "Downloaded", "Prepared", "Audited", "Using Python", "Creating virtual")),
				Yes:   []string{"Downloading numpy (16.2MiB)", "   Building foo @ file:///src", "      Built foo @ file:///src", "Prepared 3 packages in 1.2s", "Audited 5 packages in 2ms", "Using Python 3.12.1", "Creating virtual environment at: .venv"},
				No:    []string{"Downloadingx", "error: No solution found"},
			},
		},
	},
	"poetry": {
		Noise: pcomb.Rules{
			{
				Name:  "download progress",
				Match: pcomb.Re(`^\s+- (Downloading|Installing|Updating) .*\(\d+(\.\d+)?%\)`),
				Yes:   []string{"  - Downloading urllib3 (2.0.7) (45%)", "  - Installing idna (3.4): Downloading (100%)", "  - Updating x (1.0 -> 1.1) (12.5%)"},
				No:    []string{"  - Installing idna (3.4)", "Downloading x (45%)"},
			},
			{
				Name:  "plan lines",
				Match: pcomb.Prefix("Installing dependencies from lock file", "Resolving dependencies", "Writing lock file", "Updating dependencies", "Package operations: "),
				Yes:   []string{"Installing dependencies from lock file", "Resolving dependencies... (1.2s)", "Writing lock file", "Updating dependencies", "Package operations: 3 installs, 0 updates, 0 removals"},
				No:    []string{"Package operations", "Installing dependencies"},
			},
		},
		Fold: []Fold{{"installed", re(`^\s+- Installing (\S+ \(\S+\))$`)}, {"updated", re(`^\s+- Updating (\S+ \(\S+ -> \S+\))$`)}},
	},
	"go-mod": {
		Noise: pcomb.Rules{
			{
				Name:  "module downloads",
				Match: pcomb.Prefixed("go: ", pcomb.Prefix("downloading ", "finding module ", "extracting ")),
				Yes:   []string{"go: downloading golang.org/x/text v0.14.0", "go: finding module for package github.com/x/y", "go: extracting github.com/x/y v1.0.0"},
				No:    []string{"go: added golang.org/x/text v0.14.0", "go: downloading", "go: finding modules"},
			},
		},
		Fold: []Fold{{"go: added", re(`^go: added (\S+ \S+)$`)}, {"go: upgraded", re(`^go: upgraded (\S+ \S+ => \S+)$`)}},
	},
	"cargo": {
		Noise: pcomb.Rules{
			{
				Name:  "cargo progress",
				Match: pcomb.Indented(pcomb.Word("Updating", "Locking", "Downloading", "Downloaded", "Compiling", "Fetching", "Fresh", "Blocking", "Finished")),
				Yes:   []string{"    Updating crates.io index", "     Locking 12 packages", "  Downloading crates ...", "   Downloaded serde v1.0.210", "   Compiling serde v1.0.210", "    Fresh itoa v1.0.9", "    Finished `dev` profile target(s) in 3.2s"},
				No:    []string{"Compiling serde", "error: could not compile", "    Compilingly"},
			},
		},
		Fold: []Fold{{"features enabled", re(`^\s+\+ (\S+)$`)}, {"features available", re(`^\s+- (\S+)$`)}},
	},
	"os": {
		Noise: pcomb.Rules{
			{
				Name:  "apt",
				Match: pcomb.Any(pcomb.Re(`^(Get|Hit|Ign):\d+ `), pcomb.Prefix("Reading package lists", "Reading state information", "Building dependency tree", "(Reading database", "Need to get", "After this operation", "Selecting previously unselected ", "Preparing to unpack ", "Unpacking ", "Setting up ", "Processing triggers for "), pcomb.Re(`^Fetched [\d.]+ [kMG]?B`), pcomb.Re(`^\s*\d+% \[`)),
				Yes:   []string{"Get:1 http://deb.debian.org/debian bookworm InRelease [151 kB]", "Hit:2 http://security.debian.org bookworm-security InRelease", "Ign:3 http://x/y Packages", "Reading package lists... Done", "Reading state information... Done", "Building dependency tree... Done", "(Reading database ... 52341 files and directories currently installed.)", "Need to get 1,200 kB of archives.", "After this operation, 4,096 kB of additional disk space will be used.", "Selecting previously unselected package libfoo.", "Preparing to unpack .../libfoo.deb ...", "Unpacking libfoo (1.0) ...", "Setting up libfoo (1.0) ...", "Processing triggers for man-db (2.11.2-2) ...", "Fetched 1200 kB in 1s (1100 kB/s)", "Fetched 12 B in 0s", "  50% [1 libfoo 12 kB/24 kB 50%]"},
				No:    []string{"E: Unable to locate package foo", "Get:x y", "Setting up"},
			},
			{
				Name:  "brew",
				Match: pcomb.Any(pcomb.Prefixed("==> ", pcomb.Prefix("Downloading", "Fetching", "Pouring")), pcomb.Re(`^#+\s*[\d.]+%`), pcomb.Prefix("Already downloaded")),
				Yes:   []string{"==> Downloading https://ghcr.io/v2/homebrew/core/jq/manifests/1.7", "==> Fetching jq", "==> Pouring jq--1.7.arm64_sonoma.bottle.tar.gz", "######## 100.0%", "## 12.5%", "Already downloaded: /home/u/.cache/Homebrew/jq"},
				No:    []string{"==> Installing jq", "Error: jq not found"},
			},
			{
				Name:  "pacman",
				Match: pcomb.Any(pcomb.Re(`^\s*\(\s*\d+/\d+\) (checking|loading|installing|upgrading)`), pcomb.Prefixed(":: ", pcomb.Prefix("Processing", "Retrieving", "Running")), pcomb.Re(`^\s*\S+\s+[\d.]+ [KMG]iB\s+[\d.]+ [KMG]iB/s`)),
				Yes:   []string{"(1/3) checking keys in keyring", "( 2/12) installing foo", "(3/3) upgrading bar", ":: Processing package changes...", ":: Retrieving packages...", ":: Running post-transaction hooks...", "foo-1.0-1-x86_64  1.2 MiB  3.4 MiB/s 00:00"},
				No:    []string{"error: target not found: foo", ":: Proceed with installation? [Y/n]"},
			},
			{
				Name:  "dnf",
				Match: pcomb.Any(pcomb.Prefix("Downloading Packages", "Running transaction", "Transaction test", "Last metadata expiration"), pcomb.Re(`^\s*\(\d+/\d+\): `), pcomb.Re(`^\s*(Installing|Verifying|Preparing|Running scriptlet)\s+:`)),
				Yes:   []string{"Downloading Packages:", "Running transaction check", "Transaction test succeeded.", "Last metadata expiration check: 0:12:34 ago on Mon 01 May.", "(1/3): foo-1.0.rpm 1.2 MB/s | 30 kB 00:00", "  Installing       : foo-1.0-1.x86_64     1/3", "  Verifying        : foo-1.0-1.x86_64     1/3"},
				No:    []string{"Error: Unable to find a match: foo", "Installing foo"},
			},
		},
	},
	"composer": {
		Noise: pcomb.Rules{
			{
				Name:  "plan lines",
				Match: pcomb.Prefix("Loading composer repositories", "Updating dependencies", "Lock file operations", "Writing lock file", "Installing dependencies from lock file", "Generating autoload files", "Use the `composer fund`", "No security vulnerability advisories found"),
				Yes:   []string{"Loading composer repositories with package information", "Updating dependencies", "Lock file operations: 3 installs, 0 updates, 0 removals", "Writing lock file", "Installing dependencies from lock file (including require-dev)", "Generating autoload files", "Use the `composer fund` command to find out more!", "No security vulnerability advisories found."},
				No:    []string{"Your requirements could not be resolved", "Use the composer fund"},
			},
			{
				Name:  "progress",
				Match: pcomb.Any(pcomb.Indented(pcomb.Prefix("- Downloading ")), pcomb.Re(`^\s+\d+/\d+ \[[=>\-]+\]`)),
				Yes:   []string{"  - Downloading symfony/console (v6.4.0)", "   3/12 [==>------------]  25%"},
				No:    []string{"- Downloading x", "  - Installing x (1.0)"},
			},
			{
				Name:  "funding",
				Match: pcomb.Re(`^\d+ packages you are using are looking for funding`),
				Yes:   []string{"12 packages you are using are looking for funding."},
				No:    []string{"packages you are using are looking for funding"},
			},
		},
		Fold: []Fold{{"installed", re(`^\s+- Installing (\S+ \([^)]+\))(?:: .*)?$`)}, {"updated", re(`^\s+- Upgrading (\S+ \([^)]+\))(?:: .*)?$`)}},
	},
	"gem": {
		Noise: pcomb.Rules{
			{
				Name:  "bundler chatter",
				Match: pcomb.Any(pcomb.Prefix("Fetching gem metadata from", "Resolving dependencies", "Parsing documentation for", "Installing ri documentation", "Done installing documentation", "Building native extensions", "Use `bundle info"), pcomb.Re(`^Fetching \S+ [\d.]+$`)),
				Yes:   []string{"Fetching gem metadata from https://rubygems.org/.........", "Resolving dependencies...", "Parsing documentation for rake-13.0.6", "Installing ri documentation for rake-13.0.6", "Done installing documentation for rake after 0 seconds", "Building native extensions. This could take a while...", "Use `bundle info [gemname]` to see where a bundled gem is installed.", "Fetching rake 13.0.6"},
				No:    []string{"Fetching rake 13.0.6 (x)", "Could not find gem foo"},
			},
		},
		Fold: []Fold{{"Using", re(`^Using (\S+ [\d.]+)`)}, {"Installing", re(`^Installing (\S+ [\d.]+)$`)}},
	},
	"mix": {
		Noise: pcomb.Rules{
			{
				Name:  "hex chatter",
				Match: pcomb.Prefix("Resolving Hex dependencies", "Resolution completed", "Unchanged:", "Network issue"),
				Yes:   []string{"Resolving Hex dependencies...", "Resolution completed in 0.2s", "Unchanged:", "Network issue"},
				No:    []string{"* Getting phoenix (Hex package)", "Resolution"},
			},
		},
		Fold: []Fold{{"fetched", re(`^\* Getting (\S+) `)}},
	},
	"pub": {
		Noise: pcomb.Rules{
			{
				Name:  "pub chatter",
				Match: pcomb.Any(pcomb.Prefix("Resolving dependencies", "Downloading packages"), pcomb.Re(`^\s+\.\.\.$`)),
				Yes:   []string{"Resolving dependencies...", "Downloading packages...", "  ..."},
				No:    []string{"+ http 1.2.0", "..."},
			},
		},
		Fold: []Fold{{"added", re(`^\+ (\S+ \S+)(?: \(.*\))?$`)}},
	},
	"swift": {
		Noise: pcomb.Rules{
			{
				Name:  "swiftpm fetch",
				Match: pcomb.Word("Fetching", "Fetched", "Computing version for", "Computed", "Creating working copy", "Working copy of", "Updating https", "Updated https", "Cloning"),
				Yes:   []string{"Fetching https://github.com/apple/swift-log from cache", "Fetched https://github.com/apple/swift-log (0.4s)", "Computing version for https://github.com/apple/swift-log", "Computed https://github.com/apple/swift-log at 1.5.3 (0.1s)", "Creating working copy of package 'swift-log'", "Working copy of https://github.com/x/y resolved at main", "Updating https://github.com/x/y", "Updated https://github.com/x/y (0.2s)", "Cloning https://github.com/x/y"},
				No:    []string{"error: Fetching failed", "Fetchingly"},
			},
		},
	},
	"nuget": {
		Noise: pcomb.Rules{
			{
				Name:  "http cache",
				Match: pcomb.Trimmed(pcomb.Prefix("GET https://", "OK https://", "CACHE https://")),
				Yes:   []string{"  GET https://api.nuget.org/v3/index.json", "  OK https://api.nuget.org/v3/index.json 120ms", "CACHE https://api.nuget.org/v3-flatcontainer/x/index.json"},
				No:    []string{"  GET http://x", "error NU1101: Unable to find package"},
			},
			{
				Name:  "restore chatter",
				Match: pcomb.Any(pcomb.Indented(pcomb.Prefix("Determining projects", "Restored ", "Restore completed in", "Writing assets file", "Lock file has not changed", "https://api.nuget.org")), pcomb.Re(`^\s+Installed \d+ packages? in`), pcomb.Trimmed(pcomb.Prefix("Feeds used:"))),
				Yes:   []string{"  Determining projects to restore...", "  Restored /src/App.csproj (in 300 ms).", "  Restore completed in 1.2 sec for /src/App.csproj.", "  Writing assets file to disk. Path: /src/obj/project.assets.json", "  Lock file has not changed. Skipping lock file write.", "  https://api.nuget.org/v3/index.json", "  Installed 3 packages in 120 ms", "Feeds used:", "  Feeds used:"},
				No:    []string{"Restored /src/App.csproj", "  Installed packages"},
			},
		},
	},
	"conda": {
		Noise: pcomb.Rules{
			{
				Name:  "solver chatter",
				Match: pcomb.Prefix("Collecting package metadata", "Solving environment", "Preparing transaction", "Verifying transaction", "Retrieving notices", "Downloading and Extracting Packages"),
				Yes:   []string{"Collecting package metadata (current_repodata.json): done", "Solving environment: done", "Preparing transaction: done", "Verifying transaction: done", "Retrieving notices: ...working... done", "Downloading and Extracting Packages"},
				No:    []string{"PackagesNotFoundError: The following packages are not available", "Solving"},
			},
			{
				Name:  "finished download rows",
				Match: pcomb.Re(`^\S+\s+\|\s+[\d.]+ [kKMG]?B\s+\|\s+#+\s+\|\s+100%`),
				Yes:   []string{"numpy-1.26.0         | 7.5 MB    | ##################################### | 100%", "zlib-1.3             | 90 KB     | ##### | 100%"},
				No:    []string{"numpy-1.26.0 | 7.5 MB | ### | 50%"},
			},
		},
	},
	"deno": {
		Noise: pcomb.Rules{
			{
				Name:  "fetch and check",
				Match: pcomb.Any(pcomb.Prefix("Download http://", "Download https://", "Check http://", "Check https://"), pcomb.Re(`^Initialize @\S+`)),
				Yes:   []string{"Download https://deno.land/std@0.200.0/path/mod.ts", "Check https://x/main.ts", "Initialize @deno/x@1.0.0"},
				No:    []string{"Check file:///src/main.ts", "error: Module not found", "Initialize @"},
			},
		},
	},
	"nix": {
		Noise: pcomb.Rules{
			{
				Name:  "build chatter",
				Match: pcomb.Any(pcomb.Prefix("copying path", "downloading '", "waiting for", "unpacking", "checking outputs", "post-installation fixup", "patching script"), pcomb.Re(`^building '/nix/store/[^']+\.drv'\.\.\.`), pcomb.Re(`^\s*\d+(\.\d+)? [KMG]iB / `)),
				Yes:   []string{"copying path '/nix/store/abc-hello-2.12' from 'https://cache.nixos.org'...", "downloading 'https://x/y.tar.gz'...", "building '/nix/store/abc-hello-2.12.drv'...", "waiting for a machine to build", "unpacking sources", "checking outputs", "post-installation fixup", "patching script interpreter paths", "  12.5 MiB / 40.0 MiB"},
				No:    []string{"error: builder for failed", "building hello", "12 MiB"},
			},
		},
	},
	"flatpak": {
		Noise: pcomb.Rules{
			{
				Name:  "progress",
				Match: pcomb.Any(pcomb.Prefix("Installing…", "Updating…", "Fetching", "Mounting", "Setup snap", "Run configure hook"), pcomb.Re(`^\[[#=\s]+\]`), pcomb.Re(`^\s*\d+%`)),
				Yes:   []string{"Installing… ████", "Updating… 40%", "Fetching something", "Mounting x", "Setup snap \"core\" (123) security profiles", "Run configure hook of \"x\" snap", "[####    ]", "[==  ]", "  50%"},
				No:    []string{"error: No remote refs found", "Installing"},
			},
		},
	},
	"mvndeps": {
		Noise: pcomb.Rules{
			{
				Name:  "downloads",
				Match: pcomb.Any(pcomb.Prefix("Downloading from ", "Downloaded from ", "Progress ("), pcomb.Prefixed("[INFO] ", pcomb.Prefix("Download"))),
				Yes:   []string{"Downloading from central: https://repo.maven.apache.org/maven2/a/a.pom", "Downloaded from central: https://repo.maven.apache.org/maven2/a/a.pom (4 kB at 9 kB/s)", "Progress (1): 4.1/12 kB", "[INFO] Downloading from central: https://x"},
				No:    []string{"[ERROR] Downloading failed", "Download"},
			},
			{
				Name:  "separators and summary",
				Match: pcomb.Prefixed("[INFO] ", pcomb.Any(pcomb.Prefix("---", "BUILD SUCCESS", "Total time", "Finished at"), pcomb.Re(`^\s*$`), pcomb.Re(`^-+$`))),
				Yes:   []string{"[INFO] --- maven-dependency-plugin:3.6.0:tree (default-cli) @ app ---", "[INFO] ", "[INFO] ------------------------------------------------------------------------", "[INFO] BUILD SUCCESS", "[INFO] Total time:  1.2 s", "[INFO] Finished at: 2024-05-01T10:00:00+02:00"},
				No:    []string{"[ERROR] ", "[INFO] BUILD FAILURE"},
			},
			{
				Name:  "maven chatter",
				Match: pcomb.Prefixed("[INFO] ", pcomb.Any(pcomb.Prefix("Scanning", "Using auto detected"), pcomb.Re(`^Building \S+ \S+$`))),
				Yes:   []string{"[INFO] Scanning for projects...", "[INFO] Using auto detected provider org.apache.maven.surefire.junit4.JUnit4Provider", "[INFO] Building app 1.0-SNAPSHOT"},
				No:    []string{"[INFO] com.x:y:jar:1.0:compile", "[INFO] Building app"},
			},
		},
	},
}

// For returns the rules of a manager; ok is false when unknown.
func For(dialect string) (Rules, bool) {
	r, ok := rules[dialect]
	return r, ok
}

// Compress drops noise, folds lines by kind (at the first one's position)
// and squeezes blank runs.
func Compress(lines []string, r Rules) []string {
	var out []string
	at := map[string]int{}
	items := map[string][]string{}
	for _, l := range lines {
		t := strings.TrimRight(l, " \t")
		if r.Noise.Match(t) {
			continue
		}
		folded := false
		for _, f := range r.Fold {
			if m := f.Re.FindStringSubmatch(t); m != nil {
				if _, ok := at[f.Kind]; !ok {
					at[f.Kind] = len(out)
					out = append(out, "")
				}
				items[f.Kind] = append(items[f.Kind], m[1])
				folded = true
				break
			}
		}
		if !folded {
			out = append(out, t)
		}
	}
	for kind, i := range at {
		out[i] = fmt.Sprintf("%s (%d): %s", kind, len(items[kind]), strings.Join(items[kind], ", "))
	}
	return compress.SqueezeBlank(compress.DedupeRuns(trim(out)))
}

func trim(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	return lines
}
