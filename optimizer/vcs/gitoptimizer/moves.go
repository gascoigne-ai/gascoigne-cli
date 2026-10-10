package gitoptimizer

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// stretch is a run of removed or added lines in one file's hunk.
type stretch struct {
	file  string
	start int      // index of the first line in the diff
	line  int      // line number on its side (old for removed, new for added)
	text  []string // trimmed content
}

// moves replaces removed blocks that come back verbatim elsewhere in the
// diff (code moved within or across files) with one line naming the new
// place: the code is read once, where it lives now.
func (o *Optimizer) moves(lines []string) []string {
	if o.cfg.MinMoveLines == 0 {
		return lines
	}
	removed, added := runs(lines)
	out := slices.Clone(lines)
	cut := map[int]int{} // start of a moved run → its length
	for _, r := range removed {
		if len(r.text) < o.cfg.MinMoveLines || !substantial(r.text) {
			continue
		}
		for _, a := range added {
			at := index(a.text, r.text)
			if at < 0 {
				continue
			}
			where := fmt.Sprintf("%s:%d", a.file, a.line+at)
			if a.file == r.file {
				where = fmt.Sprintf("line %d", a.line+at)
			}
			note := ""
			if !sameIndent(lines[r.start:r.start+len(r.text)], lines[a.start+at:a.start+at+len(r.text)]) {
				note = ", reindented"
			}
			out[r.start] = fmt.Sprintf("-… %d lines moved to %s%s", len(r.text), where, note)
			cut[r.start] = len(r.text)
			break
		}
	}
	if len(cut) == 0 {
		return lines
	}
	var res []string
	for i := 0; i < len(out); i++ {
		res = append(res, out[i])
		if n, ok := cut[i]; ok {
			i += n - 1
		}
	}
	return res
}

// runs collects removed and added stretches, with line numbers from the
// hunk headers.
func runs(lines []string) (removed, added []stretch) {
	file := ""
	oldLine, newLine := 0, 0
	inHunk := false
	for i := 0; i < len(lines); {
		l := lines[i]
		switch {
		case strings.HasPrefix(l, "diff --git "):
			f := strings.Fields(l)
			file, inHunk = strings.TrimPrefix(f[len(f)-1], "b/"), false
		case strings.HasPrefix(l, "@@ "):
			oldLine, newLine = hunkStart(l)
			inHunk = true
		case inHunk && (strings.HasPrefix(l, "-") || strings.HasPrefix(l, "+")):
			sign := l[0]
			r := stretch{file: file, start: i}
			if sign == '-' {
				r.line = oldLine
			} else {
				r.line = newLine
			}
			for ; i < len(lines) && len(lines[i]) > 0 && lines[i][0] == sign; i++ {
				r.text = append(r.text, strings.TrimSpace(lines[i][1:]))
			}
			if sign == '-' {
				oldLine += len(r.text)
				removed = append(removed, r)
			} else {
				newLine += len(r.text)
				added = append(added, r)
			}
			continue
		case inHunk && !strings.HasPrefix(l, `\`):
			oldLine++
			newLine++
		}
		i++
	}
	return removed, added
}

// hunkStart reads "@@ -12,5 +14,7 @@" as 12 and 14.
func hunkStart(h string) (oldLine, newLine int) {
	f := strings.Fields(h)
	if len(f) < 3 {
		return 0, 0
	}
	num := func(s string) int {
		s, _, _ = strings.Cut(s[1:], ",")
		n, _ := strconv.Atoi(s)
		return n
	}
	return num(f[1]), num(f[2])
}

// substantial: not just braces and blank lines, which repeat everywhere.
func substantial(text []string) bool {
	n := 0
	for _, t := range text {
		if len(t) > 3 {
			n += len(t)
		}
	}
	return n >= 80
}

// index finds needle as a contiguous run in hay.
func index(hay, needle []string) int {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if slices.Equal(hay[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

func sameIndent(a, b []string) bool {
	for i := range a {
		if a[i][1:] != b[i][1:] {
			return false
		}
	}
	return true
}
