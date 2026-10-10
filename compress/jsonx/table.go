package jsonx

import (
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/lazyre"
)

const keyCol = "_key"

// table turns an array with at least TabulateMin similar objects into
// {"_cols": [...], "_rows": [[...], ...]}. Non-object items (omission
// markers) stay in place as rows. Items with nested objects are also tried
// flattened to dotted columns; the smaller table wins.
func (c *crusher) table(items []any) (map[string]any, bool) {
	t, ok := c.plainTable(items)
	if !nested(items) {
		return t, ok
	}
	flat := make([]any, len(items))
	for i, it := range items {
		if obj, isObj := it.(map[string]any); isObj {
			m := map[string]any{}
			Flatten(m, "", obj)
			it = m
		}
		flat[i] = it
	}
	if ft, fok := c.plainTable(flat); fok && (!ok || size(ft) < size(t)) {
		return ft, true
	}
	return t, ok
}

// plainTable tabulates items as they are.
func (c *crusher) plainTable(items []any) (map[string]any, bool) {
	var objs []map[string]any
	for _, it := range items {
		if obj, ok := it.(map[string]any); ok {
			objs = append(objs, obj)
		}
	}
	cols, ok := c.columns(objs)
	if !ok {
		return nil, false
	}
	rows := make([]any, 0, len(items))
	for _, it := range items {
		obj, isObj := it.(map[string]any)
		if !isObj {
			rows = append(rows, it)
			continue
		}
		row := make([]any, len(cols))
		for i, col := range cols {
			row[i] = obj[col]
		}
		rows = append(rows, row)
	}
	return hoist(cols, rows), true
}

// tabulateMap turns a map whose values are similar objects (e.g. lockfile
// "packages") into a table with the map key as first column.
func (c *crusher) tabulateMap(m map[string]any) any {
	if len(m) < c.cfg.TabulateMin {
		return m
	}
	keys := sortedKeys(m)
	objs := make([]map[string]any, 0, len(m))
	for _, k := range keys {
		obj, ok := m[k].(map[string]any)
		if !ok {
			return m
		}
		objs = append(objs, obj)
	}
	cols, ok := c.columns(objs)
	if !ok {
		return m
	}
	rows := make([]any, len(keys))
	for i, k := range keys {
		row := []any{k}
		for _, col := range cols {
			row = append(row, objs[i][col])
		}
		rows[i] = row
	}
	return hoist(append([]string{keyCol}, cols...), rows)
}

// columns is the sorted key union, when objects share enough of it to make
// a table cheaper than repeating keys.
func (c *crusher) columns(objs []map[string]any) ([]string, bool) {
	if len(objs) < c.cfg.TabulateMin {
		return nil, false
	}
	union := map[string]bool{}
	for _, obj := range objs {
		for k := range obj {
			union[k] = true
		}
	}
	// Too sparse when many objects use few columns: nulls would cost more
	// than repeated keys save. A few odd ones (a lockfile root) are fine.
	sparse := 0
	for _, obj := range objs {
		if 4*len(obj) < len(union) {
			sparse++
		}
	}
	return sortedKeys(union), 10*sparse <= len(objs)
}

// hoist moves columns equal in every row to "_same" and shared string
// prefixes/suffixes to "_affix".
func hoist(cols []string, rows []any) map[string]any {
	var objRows [][]any
	for _, r := range rows {
		if row, ok := r.([]any); ok {
			objRows = append(objRows, row)
		}
	}
	same := map[string]any{}
	affix := map[string]any{}
	var keepCols []int
	for c, name := range cols {
		if name != keyCol && len(objRows) > 1 && allEqual(objRows, c) {
			if objRows[0][c] != nil {
				same[name] = objRows[0][c]
			}
			continue
		}
		if pre, suf := sharedAffix(objRows, c); len(pre)+len(suf) >= 8 {
			affix[name] = []string{pre, suf}
			for _, row := range objRows {
				if s, ok := row[c].(string); ok {
					row[c] = s[len(pre) : len(s)-len(suf)]
				}
			}
		}
		keepCols = append(keepCols, c)
	}
	outCols := make([]string, len(keepCols))
	for i, c := range keepCols {
		outCols[i] = cols[c]
	}
	outRows := make([]any, len(rows))
	for i, r := range rows {
		row, ok := r.([]any)
		if !ok {
			outRows[i] = r
			continue
		}
		cells := make([]any, len(keepCols))
		for j, c := range keepCols {
			cells[j] = row[c]
		}
		outRows[i] = cells
	}
	t := map[string]any{"_cols": outCols, "_rows": outRows}
	if len(same) > 0 {
		t["_same"] = same
	}
	if len(affix) > 0 {
		t["_affix"] = affix
	}
	return t
}

func allEqual(rows [][]any, c int) bool {
	return !slices.ContainsFunc(rows[1:], func(r []any) bool { return !reflect.DeepEqual(r[c], rows[0][c]) })
}

// sharedAffix returns the prefix and suffix shared by all string cells of
// column c; empty unless every non-null cell is a string longer than both.
func sharedAffix(rows [][]any, c int) (string, string) {
	if len(rows) < 2 {
		return "", ""
	}
	var vals []string
	for _, r := range rows {
		switch v := r[c].(type) {
		case nil: // missing cells stay null
		case string:
			vals = append(vals, v)
		default:
			return "", ""
		}
	}
	if len(vals) < 2 {
		return "", ""
	}
	pre, suf := vals[0], vals[0]
	for _, v := range vals[1:] {
		for !strings.HasPrefix(v, pre) {
			pre = pre[:len(pre)-1]
		}
		for !strings.HasSuffix(v, suf) {
			suf = suf[1:]
		}
	}
	// Cut at separators so cells stay whole tokens ("12:00" not "2:00").
	for pre != "" && !isSep(pre[len(pre)-1]) {
		pre = pre[:len(pre)-1]
	}
	for suf != "" && !isSep(suf[0]) {
		suf = suf[1:]
	}
	for _, v := range vals {
		if len(pre)+len(suf) > len(v) {
			return pre, "" // overlapping affixes: keep the prefix only
		}
	}
	return pre, suf
}

func isSep(c byte) bool {
	return c < utf8.RuneSelf && !unicode.IsLetter(rune(c)) && !unicode.IsDigit(rune(c))
}

// hashLike matches checksums and ids: "sha512-…", "h1:…=", long hex/base64.
var hashLike = lazyre.New(`^([A-Za-z0-9]{1,10}[:-])?[A-Za-z0-9+/=_-]{40,}$`)

func (c *crusher) str(s string) any {
	if c.cfg.ShortenHashes && len(s) >= 40 && !strings.ContainsAny(s, " .") && hashLike.MatchString(s) && digits(s) >= 4 {
		return s[:min(len(s), 16)] + "…"
	}
	return compress.TruncateLine(s, c.cfg.MaxString)
}

func digits(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
