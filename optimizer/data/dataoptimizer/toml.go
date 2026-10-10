package dataoptimizer

import (
	"fmt"
	"strings"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/lazyre"
)

var (
	arrayTable = lazyre.New(`^\[\[([\w.-]+)\]\]\s*$`)
	subTable   = lazyre.New(`^\[\[?([\w.-]+)\]\]?\s*$`)
	keyValue   = lazyre.New(`^([\w.-]+|"[^"]+")\s*=\s*(.*)$`)
	quoted     = lazyre.New(`"([^"]*)"`)
	nameField  = lazyre.New(`name\s*=\s*"([^"]+)"`)
	// Keys of lockfile entries that only machines read.
	lockNoise = lazyre.New(`^(checksum|hash|hashes|files|wheels|sdist|source|content-hash|resolution-markers|upload-time|size|url)$`)
)

type entry struct {
	fields []string
	deps   []string
}

// toml folds repeated array tables (lockfile [[package]] entries) to one line
// each: scalar fields and dependency names, without hashes and URLs.
func (o *Optimizer) toml(text string) string {
	lines := compress.Lines(text)
	count := map[string]int{}
	for _, l := range lines {
		if m := arrayTable.FindStringSubmatch(l); m != nil && !strings.Contains(m[1], ".") {
			count[m[1]]++
		}
	}
	var out []string
	var cur *entry
	table, sub := "", ""
	flush := func() {
		if cur != nil {
			l := "  " + strings.Join(cur.fields, " ")
			if len(cur.deps) > 0 {
				l += " deps: " + strings.Join(cur.deps, ", ")
			}
			out = append(out, l)
			cur = nil
		}
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if m := arrayTable.FindStringSubmatch(l); m != nil && count[m[1]] >= 3 {
			flush()
			if m[1] != table {
				table = m[1]
				out = append(out, fmt.Sprintf("[[%s]] ×%d", table, count[table]))
			}
			cur, sub = &entry{}, ""
			continue
		}
		if m := subTable.FindStringSubmatch(l); m != nil {
			if cur != nil && strings.HasPrefix(m[1], table+".") {
				sub = strings.TrimPrefix(m[1], table+".")
				continue
			}
			flush()
			table = ""
		}
		if cur == nil {
			out = append(out, l)
			continue
		}
		m := keyValue.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		key, value := strings.Trim(m[1], `"`), m[2]
		// Multi-line arrays and inline tables continue to their closing bracket.
		for open := strings.Count(value, "[") + strings.Count(value, "{") - strings.Count(value, "]") - strings.Count(value, "}"); open > 0 && i+1 < len(lines); open = strings.Count(value, "[") + strings.Count(value, "{") - strings.Count(value, "]") - strings.Count(value, "}") {
			i++
			value += strings.TrimSpace(lines[i])
		}
		switch {
		case sub == "dependencies":
			cur.deps = append(cur.deps, key) // poetry: [package.dependencies] name = "constraint"
		case sub != "" || lockNoise.MatchString(key):
		case key == "dependencies":
			cur.deps = append(cur.deps, depNames(value)...)
		case strings.HasPrefix(value, "[") || strings.HasPrefix(value, "{"):
		case key == "name" || key == "version":
			cur.fields = append(cur.fields, strings.Trim(value, `"`))
		default:
			cur.fields = append(cur.fields, key+"="+strings.Trim(value, `"`))
		}
	}
	flush()
	return compress.Join(compress.SqueezeBlank(out))
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
