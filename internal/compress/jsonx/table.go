package jsonx

import (
	"cmp"
	"reflect"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
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
		switch v := it.(type) {
		case map[string]any:
			objs = append(objs, v)
		case []any:
			return nil, false // would read as a row, and hoist indexes it by column
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
func (c *crusher) tabulateMap(object map[string]any) any {
	if len(object) < c.cfg.TabulateMin {
		return object
	}
	keys := sortedKeys(object)
	objs := make([]map[string]any, 0, len(object))
	for _, k := range keys {
		obj, ok := object[k].(map[string]any)
		if !ok {
			return object
		}
		objs = append(objs, obj)
	}
	cols, ok := c.columns(objs)
	if !ok {
		return object
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
	nulls := func(column int) (n int) {
		for _, row := range objRows {
			if row[column] == nil {
				n++
			}
		}
		return n
	}
	// Sparse columns last, so rows can end before their trailing nulls.
	slices.SortStableFunc(keepCols, func(left, right int) int { return cmp.Compare(nulls(left), nulls(right)) })
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
		outRows[i] = trimNulls(cells)
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

// trimNulls drops trailing nulls: a row shorter than _cols has null cells at the end.
func trimNulls(cells []any) []any {
	for len(cells) > 0 && cells[len(cells)-1] == nil {
		cells = cells[:len(cells)-1]
	}
	return cells
}

func allEqual(rows [][]any, column int) bool {
	return !slices.ContainsFunc(rows[1:], func(row []any) bool { return !reflect.DeepEqual(row[column], rows[0][column]) })
}

// sharedAffix returns the prefix and suffix shared by all string cells of
// column c; empty unless every non-null cell is a string longer than both.
func sharedAffix(rows [][]any, column int) (string, string) {
	if len(rows) < 2 {
		return "", ""
	}
	var vals []string
	for _, r := range rows {
		switch v := r[column].(type) {
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

func isSep(char byte) bool {
	return char < utf8.RuneSelf && !unicode.IsLetter(rune(char)) && !unicode.IsDigit(rune(char))
}

// hashLike matches checksums and ids: "sha512-…", "h1:…=", long hex/base64.
var hashLike = lazyre.New(`^([A-Za-z0-9]{1,10}[:-])?[A-Za-z0-9+/=_-]{40,}$`)

func (c *crusher) str(text string) any {
	if c.cfg.ShortenHashes && len(text) >= 40 && !strings.ContainsAny(text, " .") && hashLike.MatchString(text) && digits(text) >= 4 {
		return text[:min(len(text), 16)] + "…"
	}
	return compress.TruncateLine(text, c.cfg.MaxString)
}

func digits(text string) int {
	n := 0
	for _, r := range text {
		if unicode.IsDigit(r) {
			n++
		}
	}
	return n
}

func sortedKeys[V any](entries map[string]V) []string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
