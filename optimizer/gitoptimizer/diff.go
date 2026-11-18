package gitoptimizer

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// diff shrinks unified diffs: one header line per file, index lines dropped,
// context trimmed to ContextLines around changes, per-file size capped.
// Commit headers (git show) collapse like log entries.
func (o *Optimizer) diff(lines []string) []string {
	if len(lines) > 0 && strings.HasPrefix(lines[0], "commit ") {
		end := 0
		for end < len(lines) && !strings.HasPrefix(lines[end], "diff --git ") {
			end++
		}
		return append(o.log(lines[:end]), o.diff(lines[end:])...)
	}
	if n := statEnd(lines); n > 0 {
		return append(statLines(lines[:n]), o.diff(lines[n:])...)
	}
	lines = o.moves(lines)
	var out, file []string
	flush := func() {
		out = append(out, o.file(file)...)
		file = nil
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "diff --git ") || strings.HasPrefix(l, "diff --cc ") {
			flush()
		}
		file = append(file, l)
	}
	flush()
	return out
}

var statSummary = lazyre.New(`^\s*\d+ files? changed`)

// statEnd is the length of a leading --stat block (git diff --stat, alone or
// before a patch), 0 when the lines do not start with one. It must end in the
// summary line, which keeps a stray "a | b" line from qualifying.
func statEnd(lines []string) int {
	for i, l := range lines {
		if statSummary.MatchString(l) {
			return i + 1
		}
		if !statCol.MatchString(l) {
			return 0
		}
	}
	return 0
}

func statLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = statLine(l)
	}
	return out
}

// file compresses one file's section of a diff.
func (o *Optimizer) file(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	header, i := parseFileHeader(lines)
	out := header.unknown
	if kind := machineMade(header.name, lines[i:]); kind != "" {
		added, removed := countChanges(lines[i:])
		return append(out, fmt.Sprintf("diff %s%s (%s) +%d -%d lines folded", header.name, header.note, kind, added, removed))
	}
	if header.name != "" {
		out = append(out, "diff "+header.name+header.note)
	}
	body := o.hunks(lines[i:])
	if len(body) > o.cfg.MaxFileLines {
		body = append(body[:o.cfg.MaxFileLines], compress.Omitted(len(body)-o.cfg.MaxFileLines, "diff lines"))
	}
	return append(out, body...)
}

// fileHeader is what the git header of one file's diff says: its name, a
// note on how it changed, and the header lines git does not define.
type fileHeader struct {
	name, note string
	unknown    []string
}

// parseFileHeader reads the header lines up to the first hunk and returns
// the index of that hunk.
func parseFileHeader(lines []string) (header fileHeader, hunk int) {
	for ; hunk < len(lines) && !strings.HasPrefix(lines[hunk], "@@"); hunk++ {
		l := lines[hunk]
		switch {
		case strings.HasPrefix(l, "diff --git "):
			f := strings.Fields(l)
			header.name = strings.TrimPrefix(f[len(f)-1], "b/")
		case strings.HasPrefix(l, "rename from "):
			header.name = strings.TrimPrefix(l, "rename from ") + " -> " + header.name
		case strings.HasPrefix(l, "new file mode"):
			header.note = " (new)"
		case strings.HasPrefix(l, "deleted file mode"):
			header.note = " (deleted)"
		case strings.HasPrefix(l, "Binary files"):
			header.note = " (binary)"
		case hasPrefix(l, "index ", "--- ", "+++ ", "similarity index", "rename to ", "old mode", "new mode", "dissimilarity"):
		default:
			header.unknown = append(header.unknown, l) // not a git header we know: keep
		}
	}
	return header, hunk
}

// countChanges counts the added and removed lines of a diff body.
func countChanges(lines []string) (added, removed int) {
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "+"):
			added++
		case strings.HasPrefix(l, "-"):
			removed++
		}
	}
	return added, removed
}

// hunks keeps hunk headers, changes and ContextLines of context around them;
// small in-line edits read as word diffs.
func (o *Optimizer) hunks(lines []string) []string {
	lines = wordDiff(lines)
	isChange := func(line string) bool { return hasPrefix(line, "+", "-", "~") }
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if strings.HasPrefix(l, "@@") || strings.HasPrefix(l, `\`) {
			keep[i] = true
			continue
		}
		if !isChange(l) {
			continue
		}
		for j := max(0, i-o.cfg.ContextLines); j <= min(len(lines)-1, i+o.cfg.ContextLines); j++ {
			keep[j] = true
		}
	}
	var out []string
	gap := false
	for i, l := range lines {
		if !keep[i] {
			gap = true
			continue
		}
		if gap && len(out) > 0 && !strings.HasPrefix(l, "@@") && !strings.HasPrefix(out[len(out)-1], "@@") {
			out = append(out, " …")
		}
		gap = false
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return out
}

// lockfiles are written by package managers; their diffs repeat what the
// manifest (go.mod, package.json, Cargo.toml) already shows.
var lockfiles = map[string]bool{
	"go.sum": true, "go.work.sum": true, "package-lock.json": true, "npm-shrinkwrap.json": true,
	"yarn.lock": true, "pnpm-lock.yaml": true, "bun.lock": true, "Cargo.lock": true, "poetry.lock": true,
	"uv.lock": true, "pdm.lock": true, "Pipfile.lock": true, "Gemfile.lock": true, "composer.lock": true,
	"mix.lock": true, "pubspec.lock": true, "flake.lock": true, "packages.lock.json": true, "gradle.lockfile": true,
}

// generatedSuffixes name files written by tools (protobuf, minifiers, source maps).
var generatedSuffixes = []string{".pb.go", ".pb.gw.go", "_pb2.py", "_pb2_grpc.py", ".min.js", ".min.css", ".js.map", ".css.map"}

// machineMade reports whether a file's diff is a lockfile or generated code,
// which an agent should not read line by line: "lockfile", "generated" or "".
func machineMade(name string, body []string) string {
	base := path.Base(name)
	switch {
	case lockfiles[base]:
		return "lockfile"
	case slices.ContainsFunc(generatedSuffixes, func(suffix string) bool { return strings.HasSuffix(base, suffix) }):
		return "generated"
	}
	for _, l := range body[:min(len(body), 20)] {
		if strings.Contains(l, "Code generated") && strings.Contains(l, "DO NOT EDIT") {
			return "generated"
		}
	}
	return ""
}

func hasPrefix(text string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(text, p) {
			return true
		}
	}
	return false
}
