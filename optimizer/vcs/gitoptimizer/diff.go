package gitoptimizer

import (
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
	if name != "" {
		out = append(out, "diff "+name+note)
	}
	body := o.hunks(lines[i:])
	if len(body) > o.cfg.MaxFileLines {
		body = append(body[:o.cfg.MaxFileLines], compress.Omitted(len(body)-o.cfg.MaxFileLines, "diff lines"))
	}
	return append(out, body...)
}

// hunks keeps hunk headers, changes and ContextLines of context around them.
func (o *Optimizer) hunks(lines []string) []string {
	isChange := func(l string) bool { return strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") }
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

func hasPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}
