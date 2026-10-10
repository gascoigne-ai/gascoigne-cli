package gitoptimizer

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
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

// file compresses one file's section of a diff.
func (o *Optimizer) file(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	var head []string
	name, note := "", ""
	i := 0
	for ; i < len(lines) && !strings.HasPrefix(lines[i], "@@"); i++ {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "diff --git "):
			f := strings.Fields(l)
			name = strings.TrimPrefix(f[len(f)-1], "b/")
		case strings.HasPrefix(l, "rename from "):
			name = strings.TrimPrefix(l, "rename from ") + " -> " + name
		case strings.HasPrefix(l, "new file mode"):
			note = " (new)"
		case strings.HasPrefix(l, "deleted file mode"):
			note = " (deleted)"
		case strings.HasPrefix(l, "Binary files"):
			note = " (binary)"
		case hasPrefix(l, "index ", "--- ", "+++ ", "similarity index", "rename to ", "old mode", "new mode", "dissimilarity"):
		default:
			head = append(head, l) // not a git header we know: keep
		}
	}
	out := head
	if kind := machineMade(name, lines[i:]); kind != "" {
		added, removed := 0, 0
		for _, l := range lines[i:] {
			switch {
			case strings.HasPrefix(l, "+"):
				added++
			case strings.HasPrefix(l, "-"):
				removed++
			}
		}
		return append(out, fmt.Sprintf("diff %s%s (%s) +%d -%d lines folded", name, note, kind, added, removed))
	}
	if name != "" {
		out = append(out, "diff "+name+note)
	}
	body := o.hunks(lines[i:])
	if len(body) > o.cfg.MaxFileLines {
		body = append(body[:o.cfg.MaxFileLines], compress.Omitted(len(body)-o.cfg.MaxFileLines, "diff lines"))
	}
	return append(out, body...)
}

// hunks keeps hunk headers, changes and ContextLines of context around them;
// small in-line edits read as word diffs.
func (o *Optimizer) hunks(lines []string) []string {
	lines = wordDiff(lines)
	isChange := func(l string) bool { return hasPrefix(l, "+", "-", "~") }
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
	case slices.ContainsFunc(generatedSuffixes, func(s string) bool { return strings.HasSuffix(base, s) }):
		return "generated"
	}
	for _, l := range body[:min(len(body), 20)] {
		if strings.Contains(l, "Code generated") && strings.Contains(l, "DO NOT EDIT") {
			return "generated"
		}
	}
	return ""
}

func hasPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
