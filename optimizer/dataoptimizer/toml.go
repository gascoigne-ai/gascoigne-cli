package dataoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	arrayTable = lazyre.New(`^\[\[([\w.-]+)\]\]\s*$`)
	subTable   = lazyre.New(`^\[\[?([\w.-]+)\]\]?\s*$`)
	keyValue   = lazyre.New(`^([\w.-]+|"[^"]+")\s*=\s*(.*)$`)
	quoted     = lazyre.New(`"([^"]*)"`)
	nameField  = lazyre.New(`name\s*=\s*"([^"]+)"`)
	// Keys of lockfile entries that only machines read.
	lockNoise = map[string]bool{"checksum": true, "hash": true, "hashes": true, "files": true, "wheels": true, "sdist": true, "source": true,
		"content-hash": true, "resolution-markers": true, "upload-time": true, "size": true, "url": true}
)

type entry struct {
	fields []string
	deps   []string
}

// toml folds repeated array tables (lockfile [[package]] entries) to one line
// each: scalar fields and dependency names, without hashes and URLs.
func toml(text string) string {
	lines := compress.Lines(text)
	fold := &tomlFold{lines: lines, count: arrayTableCounts(lines)}
	for fold.i = 0; fold.i < len(lines); fold.i++ {
		fold.line(lines[fold.i])
	}
	fold.flush()
	return compress.Join(compress.SqueezeBlank(fold.out))
}

// arrayTableCounts counts the top-level array tables by name.
func arrayTableCounts(lines []string) map[string]int {
	count := map[string]int{}
	for _, l := range lines {
		if m := arrayTable.FindStringSubmatch(l); m != nil && !strings.Contains(m[1], ".") {
			count[m[1]]++
		}
	}
	return count
}

// tomlFold is the state of toml: the entry being read, the array table it
// belongs to and the sub-table inside it.
type tomlFold struct {
	lines      []string
	i          int // index of the line being read; multi-line values advance it
	count      map[string]int
	out        []string
	cur        *entry
	table, sub string
}

// flush writes the entry being read as one line.
func (fold *tomlFold) flush() {
	if fold.cur == nil {
		return
	}
	l := "  " + strings.Join(fold.cur.fields, " ")
	if len(fold.cur.deps) > 0 {
		l += " deps: " + strings.Join(fold.cur.deps, ", ")
	}
	fold.out = append(fold.out, l)
	fold.cur = nil
}

// line reads one line: a table header, or a key of the current entry.
func (fold *tomlFold) line(line string) {
	if m := arrayTable.FindStringSubmatch(line); m != nil && fold.count[m[1]] >= 3 {
		fold.startEntry(m[1])
		return
	}
	if m := subTable.FindStringSubmatch(line); m != nil {
		if fold.cur != nil && strings.HasPrefix(m[1], fold.table+".") {
			fold.sub = strings.TrimPrefix(m[1], fold.table+".")
			return
		}
		fold.flush()
		fold.table = ""
	}
	if fold.cur == nil {
		fold.out = append(fold.out, line)
		return
	}
	if m := keyValue.FindStringSubmatch(line); m != nil {
		fold.keyValue(strings.Trim(m[1], `"`), m[2])
	}
}

// startEntry begins an entry of array table name, with a header line the
// first time the table appears.
func (fold *tomlFold) startEntry(name string) {
	fold.flush()
	if name != fold.table {
		fold.table = name
		fold.out = append(fold.out, fmt.Sprintf("[[%s]] ×%d", name, fold.count[name]))
	}
	fold.cur, fold.sub = &entry{}, ""
}

// keyValue adds a key of the current entry: its scalar fields and
// dependency names, nothing else.
func (fold *tomlFold) keyValue(key, value string) {
	// Multi-line arrays and inline tables continue to their closing bracket.
	for open := depth(value); open > 0 && fold.i+1 < len(fold.lines); {
		fold.i++
		next := strings.TrimSpace(fold.lines[fold.i])
		value += next
		open += depth(next)
	}
	cur := fold.cur
	switch {
	case fold.sub == "dependencies":
		cur.deps = append(cur.deps, key) // poetry: [package.dependencies] name = "constraint"
	case fold.sub != "" || lockNoise[key]:
	case key == "dependencies":
		cur.deps = append(cur.deps, depNames(value)...)
	case strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{"):
	case key == "name" || key == "version":
		cur.fields = append(cur.fields, strings.Trim(value, `"`))
	default:
		cur.fields = append(cur.fields, key+"="+strings.Trim(value, `"`))
	}
}

// depNames reads uv inline tables ({ name = "x", … }) and cargo strings
// ("serde 1.0.1").
func depNames(value string) []string {
	var out []string
	if names := nameField.FindAllStringSubmatch(value, -1); names != nil {
		for _, n := range names {
			out = append(out, n[1])
		}
		return out
	}
	for _, q := range quoted.FindAllStringSubmatch(value, -1) {
		name, _, _ := strings.Cut(q[1], " ")
		out = append(out, name)
	}
	return out
}

// depth is the net count of opened brackets and braces in s.
func depth(line string) int {
	return strings.Count(line, "[") + strings.Count(line, "{") - strings.Count(line, "]") - strings.Count(line, "}")
}
