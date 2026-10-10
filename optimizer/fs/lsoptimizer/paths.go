package lsoptimizer

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/table"
)

// fold summarizes names past max: "… 40 more: .go ×38, .md ×2".
func fold(names []string, max int) []string {
	if len(names) <= max {
		return names
	}
	return append(names[:max:max], more(names[max:]))
}

func more(names []string) string {
	exts := map[string]int{}
	for _, n := range names {
		exts[cmp.Or(path.Ext(n), "(none)")]++
	}
	keys := slices.SortedFunc(maps.Keys(exts), func(a, b string) int { return cmp.Or(cmp.Compare(exts[b], exts[a]), cmp.Compare(a, b)) })
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s ×%d", k, exts[k]))
	}
	return fmt.Sprintf("… %d more: %s", len(names), strings.Join(parts, ", "))
}

// paths groups path lists by directory: "dir/: a.go  b.go", long ones folded.
func (o *Optimizer) paths(lines []string) []string {
	var dirs []string
	files := map[string][]string{}
	isDir := map[string]bool{}
	for _, l := range lines {
		p := strings.TrimPrefix(strings.TrimSpace(l), "./")
		if p == "" || p == "." {
			continue
		}
		isDir[path.Dir(strings.TrimSuffix(p, "/"))] = true
		if strings.HasSuffix(p, "/") {
			isDir[strings.TrimSuffix(p, "/")] = true
		}
	}
	for _, l := range lines {
		p := strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(l), "./"), "/")
		if p == "" || p == "." || isDir[p] {
			continue // directories show as group headers
		}
		dir := path.Dir(p)
		if _, ok := files[dir]; !ok {
			dirs = append(dirs, dir)
		}
		files[dir] = append(files[dir], path.Base(p))
	}
	if len(dirs) == 0 {
		return lines
	}
	var out []string
	for _, d := range dirs {
		out = append(out, wrap(strings.TrimPrefix(d+"/: ", "./"), fold(files[d], o.cfg.MaxPerDir), o.cfg.Width)...)
	}
	return out
}

var treeEntry = regexp.MustCompile(`^((?:│   |│\x{a0}\x{a0} |    )*)(├──|└──) (.*)$`)

// tree indents by two spaces instead of box drawing and lists at most
// MaxPerDir files per directory.
func (o *Optimizer) tree(lines []string) []string {
	var out []string
	var files []int       // files seen per depth in the open directories
	var hidden [][]string // files past the limit per depth
	close := func(depth int) {
		for d := len(files) - 1; d >= depth; d-- {
			if len(hidden[d]) > 0 {
				out = append(out, strings.Repeat("  ", d)+more(hidden[d]))
			}
		}
		files, hidden = files[:min(depth, len(files))], hidden[:min(depth, len(hidden))]
	}
	parsed := false
	for i, l := range lines {
		m := treeEntry.FindStringSubmatch(l)
		if m == nil {
			close(0)
			out = append(out, l) // root and summary lines
			continue
		}
		parsed = true
		d := len([]rune(m[1])) / 4
		close(d + 1)
		for len(files) <= d {
			files, hidden = append(files, 0), append(hidden, nil)
		}
		next := treeEntry.FindStringSubmatch(lineAt(lines, i+1))
		if next != nil && len([]rune(next[1]))/4 > d { // a directory with entries
			out = append(out, strings.Repeat("  ", d)+m[3]+"/")
			continue
		}
		if files[d]++; files[d] > o.cfg.MaxPerDir {
			hidden[d] = append(hidden[d], m[3])
			continue
		}
		out = append(out, strings.Repeat("  ", d)+m[3])
	}
	close(0)
	if !parsed {
		return lines
	}
	return out
}

func lineAt(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

var duLine = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)([KMGTP]?)\t(.+)$`)

// du keeps the largest entries and the total (the last line).
func (o *Optimizer) du(lines []string) []string {
	type entry struct {
		line string
		size float64
	}
	var entries []entry
	for _, l := range lines {
		if m := duLine.FindStringSubmatch(l); m != nil {
			n, _ := strconv.ParseFloat(strings.Replace(m[1], ",", ".", 1), 64)
			entries = append(entries, entry{l, n * float64(int64(1)<<(10*strings.Index(" KMGTP", cmp.Or(m[2], " "))))})
		}
	}
	if len(entries) <= o.cfg.MaxPerDir*2 || len(entries) != len(compress.Lines(strings.Join(lines, "\n"))) {
		return lines
	}
	total := entries[len(entries)-1]
	rest := entries[:len(entries)-1]
	slices.SortStableFunc(rest, func(a, b entry) int { return cmp.Compare(b.size, a.size) })
	out := []string{"largest:"}
	for _, e := range rest[:o.cfg.MaxPerDir*2] {
		out = append(out, e.line)
	}
	return append(out, compress.Omitted(len(rest)-o.cfg.MaxPerDir*2, "smaller entries"), total.line)
}

var pseudoFS = regexp.MustCompile(`^(tmpfs|devtmpfs|udev|dev|overlay|shm|efivarfs|squashfs|/dev/loop\d+|none|run|cgroup2?|proc|sysfs|portal)$`)

// df drops pseudo filesystems and compacts the table.
func (o *Optimizer) df(lines []string) []string {
	if len(lines) < 2 {
		return lines
	}
	// Single spaces separate header words ("Use% Mounted on"): split on
	// whitespace, the mount point takes the rest.
	header := strings.Fields(strings.Replace(lines[0], "Mounted on", "Mounted-on", 1))
	t := table.Table{Header: header}
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		if len(f) < len(header) {
			return lines
		}
		t.Rows = append(t.Rows, append(f[:len(header)-1:len(header)-1], strings.Join(f[len(header)-1:], " ")))
	}
	var kept [][]string
	for _, r := range t.Rows {
		if !pseudoFS.MatchString(r[0]) && !strings.HasPrefix(r[len(r)-1], "/run/") && !strings.HasPrefix(r[len(r)-1], "/sys/") {
			kept = append(kept, r)
		}
	}
	dropped := len(t.Rows) - len(kept)
	t.Rows = kept
	out := table.Render(t, table.Options{MaxRows: 40, MaxCell: 60})
	if dropped > 0 {
		out = append(out, compress.Omitted(dropped, "pseudo filesystem rows"))
	}
	return out
}
