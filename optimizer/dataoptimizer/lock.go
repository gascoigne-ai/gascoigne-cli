package dataoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	yarnHeader = lazyre.New(`^"?(@?[^@\s"]+)@[^\s"]*`)
	yarnVer    = lazyre.New(`^  version:? "?([^"\s]+)"?$`)
	goSumLine  = lazyre.New(`^(\S+) (v\S+?)(/go\.mod)? h1:\S+$`)
	gemSpec    = lazyre.New(`^    (\S+) \(([^)]+)\)$`)
)

// lock lists the packages of a text lockfile (yarn.lock, go.sum,
// Gemfile.lock) as name and version, without hashes, URLs and dependency
// edges. Text of another shape stays as it is.
func lock(text string) string {
	lines := compress.Lines(text)
	var out []string
	switch {
	case goSumLine.MatchString(firstText(lines)):
		out = goSum(lines)
	case strings.Contains(text, "\nGEM\n") || strings.HasPrefix(text, "GEM\n"):
		out = gemfileLock(lines)
	case strings.Contains(text, "# yarn lockfile"):
		out = yarnLock(lines)
	default:
		return text
	}
	if res := compress.Join(out); len(res) < len(text) {
		return res
	}
	return text
}

func firstText(lines []string) string {
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}

// goSum keeps each module version once; a /go.mod hash line is the same module.
func goSum(lines []string) []string {
	seen := map[string]bool{}
	var mods []string
	for _, l := range lines {
		if m := goSumLine.FindStringSubmatch(l); m != nil && !seen[m[1]+" "+m[2]] {
			seen[m[1]+" "+m[2]] = true
			mods = append(mods, m[1]+" "+m[2])
		}
	}
	return append([]string{fmt.Sprintf("go.sum: %d module versions (hashes dropped)", len(mods))}, mods...)
}

// yarnLock lists name@version once per entry.
func yarnLock(lines []string) []string {
	seen := map[string]bool{}
	var pkgs []string
	name := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "#") || strings.TrimSpace(l) == "" {
			continue
		}
		if !strings.HasPrefix(l, " ") {
			name = ""
			if m := yarnHeader.FindStringSubmatch(l); m != nil {
				name = m[1]
			}
			continue
		}
		if m := yarnVer.FindStringSubmatch(l); m != nil && name != "" && !seen[name+"@"+m[1]] {
			seen[name+"@"+m[1]] = true
			pkgs = append(pkgs, name+"@"+m[1])
		}
	}
	return append([]string{fmt.Sprintf("yarn.lock: %d packages (resolved URLs, integrity hashes and dependency lists dropped)", len(pkgs))}, pkgs...)
}

// gemfileLock keeps every section and lists the gems of specs as name version.
func gemfileLock(lines []string) []string {
	var out []string
	for _, l := range lines {
		if m := gemSpec.FindStringSubmatch(l); m != nil {
			out = append(out, "  "+m[1]+" "+m[2])
			continue
		}
		if strings.HasPrefix(l, "      ") { // a gem's dependency edge
			continue
		}
		out = append(out, l)
	}
	return out
}
