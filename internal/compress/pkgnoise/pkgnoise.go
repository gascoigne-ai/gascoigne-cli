// Package pkgnoise compresses package manager output: progress, downloads,
// cache hits and boilerplate go; repeated warnings fold into one line per
// kind; summaries, added/removed packages, vulnerabilities and errors stay.
package pkgnoise

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Rules describe a manager.
type Rules struct {
	Noise *lazyre.Regexp // dropped
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
		Noise: re("^npm (http|sill|silly|verb|verbose|timing|notice|info)\\b|^\\d+ packages? (are|is) looking for funding|^\\s*run `npm fund` for details|" +
			"^To address (all )?issues|^\\s*npm audit fix|^Some issues need review|^a different dependency|^Run `npm audit` for details"),
		Fold: []Fold{{"npm warn deprecated", re(`^npm (?:warn|WARN) deprecated (\S+?):`)}},
	},
	"pnpm": {
		Noise: re(`^Progress: resolved|^[+-]{5,}$|^Downloading |^Done in [\d.]+m?s|^\s*╭|^\s*╰|^\s*│\s*$`),
		Fold:  []Fold{{"WARN deprecated", re(`^\s*WARN\s+deprecated (\S+?):`)}, {"WARN deprecated subdependencies", re(`^\s*WARN\s+\d+ deprecated subdependencies found: (.+)$`)}},
	},
	"yarn": {
		Noise: re(`^\[\d/\d\] (Resolving|Fetching|Linking|Building)|^info (No lockfile|Visit|There appears|fsevents)|^success Saved lockfile|^➤ YN0000: [┌└│]|^➤ YN0013|^yarn install v|^Done in [\d.]+s`),
		Fold:  []Fold{{"warning deprecated", re(`^warning (\S+?): .*deprecated`)}, {"unmet peer dependency", re(`^warning "(\S+?)" has (?:unmet|incorrect) peer dependency`)}},
	},
	"bun": {
		Noise: re(`^\[\d+(\.\d+)?m?s\] |^Extracted |^Resolving dependencies|^Resolved, downloaded and extracted|^Saved lockfile`),
	},
	"pip": {
		Noise: re(`^Collecting |^\s+Downloading |^\s+Using cached |^\s+━|^Requirement already satisfied|^Installing collected packages|^\s+Attempting uninstall|^\s+Found existing installation|^\s+Uninstalling |^\s+Successfully uninstalled|^Building wheels? for|^\s+Building wheel for|^\s+Created wheel for|^\s+Stored in directory|^\s*Preparing metadata|^Obtaining |^\s+Installing build dependencies|^\s+Getting requirements to build|^Downloading |^Using cached `),
	},
	"uv": {
		Noise: re(`^\s*(Downloading|Building|Built|Downloaded|Prepared|Audited|Using Python|Creating virtual)\b`),
	},
	"poetry": {
		Noise: re(`^\s+- (Downloading|Installing|Updating) .*\(\d+(\.\d+)?%\)|^Installing dependencies from lock file|^Resolving dependencies|^Writing lock file|^Updating dependencies|^Package operations: `),
		Fold:  []Fold{{"installed", re(`^\s+- Installing (\S+ \(\S+\))$`)}, {"updated", re(`^\s+- Updating (\S+ \(\S+ -> \S+\))$`)}},
	},
	"go-mod": {
		Noise: re(`^go: (downloading|finding module|extracting) `),
		Fold:  []Fold{{"go: added", re(`^go: added (\S+ \S+)$`)}, {"go: upgraded", re(`^go: upgraded (\S+ \S+ => \S+)$`)}},
	},
	"cargo": {
		Noise: re(`^\s+(Updating|Locking|Downloading|Downloaded|Compiling|Fetching|Fresh|Blocking|Finished)\b`),
		Fold:  []Fold{{"features enabled", re(`^\s+\+ (\S+)$`)}, {"features available", re(`^\s+- (\S+)$`)}},
	},
	"os": {
		Noise: re(`^(Get|Hit|Ign):\d+ |^Reading (package lists|state information)|^Building dependency tree|^(Selecting previously unselected|Preparing to unpack|Unpacking|Setting up|Processing triggers for) |^\(Reading database|^Fetched [\d.]+ [kMG]?B|^Need to get|^After this operation|^\s*\d+% \[` +
			`|^==> (Downloading|Fetching|Pouring)|^#+\s*[\d.]+%|^Already downloaded` + // brew
			`|^\s*\(\s*\d+/\d+\) (checking|loading|installing|upgrading)|^:: (Processing|Retrieving|Running)|^\s*\S+\s+[\d.]+ [KMG]iB\s+[\d.]+ [KMG]iB/s` + // pacman
			`|^(Downloading Packages|Running transaction|Transaction test|Last metadata expiration)|^\s*\(\d+/\d+\): |^\s*(Installing|Verifying|Preparing|Running scriptlet)\s+:`), // dnf
	},
	"composer": {
		Noise: re(`^Loading composer repositories|^Updating dependencies|^Lock file operations|^Writing lock file|^Installing dependencies from lock file|^Generating autoload files|^\s+- Downloading |^\s+\d+/\d+ \[[=>\-]+\]|^\d+ packages you are using are looking for funding|^Use the \x60composer fund\x60|^No security vulnerability advisories found`),
		Fold:  []Fold{{"installed", re(`^\s+- Installing (\S+ \([^)]+\))(?:: .*)?$`)}, {"updated", re(`^\s+- Upgrading (\S+ \([^)]+\))(?:: .*)?$`)}},
	},
	"gem": {
		Noise: re(`^Fetching gem metadata from|^Resolving dependencies|^Fetching \S+ [\d.]+$|^Parsing documentation for|^Installing ri documentation|^Done installing documentation|^Building native extensions|^Use \x60bundle info`),
		Fold:  []Fold{{"Using", re(`^Using (\S+ [\d.]+)`)}, {"Installing", re(`^Installing (\S+ [\d.]+)$`)}},
	},
	"mix": {
		Noise: re(`^Resolving Hex dependencies|^Resolution completed|^Unchanged:|^Network issue`),
		Fold:  []Fold{{"fetched", re(`^\* Getting (\S+) `)}},
	},
	"pub": {
		Noise: re(`^Resolving dependencies|^Downloading packages|^\s+\.\.\.$`),
		Fold:  []Fold{{"added", re(`^\+ (\S+ \S+)(?: \(.*\))?$`)}},
	},
	"swift": {
		Noise: re(`^(Fetching|Fetched|Computing version for|Computed|Creating working copy|Working copy of|Updating https|Updated https|Cloning)\b`),
	},
	"nuget": {
		Noise: re(`^\s*(GET|OK|CACHE) https://|^\s+Determining projects|^\s+Restored |^\s+Restore completed in|^\s+Writing assets file|^\s+Lock file has not changed|^\s+Installed \d+ packages? in|^\s*Feeds used:|^\s+https://api\.nuget\.org`),
	},
	"conda": {
		Noise: re(`^(Collecting package metadata|Solving environment|Preparing transaction|Verifying transaction|Retrieving notices|Downloading and Extracting Packages)|^\S+\s+\|\s+[\d.]+ [kKMG]?B\s+\|\s+#+\s+\|\s+100%`),
	},
	"deno": {Noise: re(`^(Download|Check) https?://|^Initialize @\S+`)},
	"nix": {
		Noise: re(`^(copying path|downloading '|building '/nix/store/[^']+\.drv'\.\.\.|waiting for|unpacking|checking outputs|post-installation fixup|patching script)|^\s*\d+(\.\d+)? [KMG]iB / `),
	},
	"flatpak": {Noise: re(`^(Installing|Updating)…|^\[[#=\s]+\]|^\s*\d+%|^Fetching|^Mounting|^Setup snap|^Run configure hook`)},
	"mvndeps": {Noise: re(`^\[INFO\] (Download|Scanning|---|\s*$|-+$|BUILD SUCCESS|Total time|Finished at|Using auto detected|Building \S+ \S+$)|^Downloading from |^Downloaded from |^Progress \(`)},
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
		if r.Noise != nil && r.Noise.MatchString(t) {
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
