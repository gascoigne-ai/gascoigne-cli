// Package legend writes and reads the one-line dictionaries optimizers put
// on top of an output: a long repeated string is defined once and the lines
// below use a short id, so the agent (and the probe) can always expand it.
package legend

import (
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

// Kinds. Each has its own id sigil, so ids of two legends never collide.
const (
	Paths = "paths"
	Rules = "rules"
	Names = "names"
)

var sigil = map[string]string{Paths: "§", Rules: "¶", Names: "†"}

// Def is one legend entry: Value stands behind the id.
type Def struct{ Id, Value string }

var lineRe = lazyre.New(`\[(paths|rules|names): ([^\]\n]+)\]`)

// ID is the short id of the i-th entry (from 0) of a kind: §1, ¶2, †3.
func ID(kind string, i int) string { return sigil[kind] + strconv.Itoa(i+1) }

// Render is the legend line for defs, without a newline.
func Render(kind string, defs []Def) string {
	parts := make([]string, len(defs))
	for i, d := range defs {
		parts[i] = d.Id + "=" + d.Value
	}
	return "[" + kind + ": " + strings.Join(parts, " ") + "]"
}

// Is reports a legend line: the lines below use its ids, so it always stays.
func Is(line string) bool {
	loc := lineRe.FindStringIndex(line)
	return loc != nil && loc[0] == 0
}

// Has reports whether text carries a legend line anywhere.
func Has(text string) bool { return lineRe.MatchString(text) }

// Worth is the bytes saved by replacing n uses of value with id, the legend
// entry (id=value and a separator) counted. Negative means a loss.
func Worth(n, valueLen, idLen int) int {
	return n*(valueLen-idLen) - idLen - valueLen - 2
}

// Expand puts every id of every legend in text back to its value. Legend
// lines stay, so the text still reads as it was written.
func Expand(text string) string {
	var defs []Def
	for _, m := range lineRe.FindAllStringSubmatch(text, -1) {
		defs = append(defs, parse(m[2])...)
	}
	if len(defs) == 0 {
		return text
	}
	// Longest ids first: §12 must not be read as §1 followed by 2.
	slices.SortStableFunc(defs, func(a, b Def) int { return len(b.Id) - len(a.Id) })
	pairs := make([]string, 0, 2*len(defs))
	for _, d := range defs {
		pairs = append(pairs, d.Id, d.Value)
	}
	return strings.NewReplacer(pairs...).Replace(text)
}

// parse splits "id=value id=value". A value holds spaces, so an entry ends
// where the next id starts: a sigil or a source letter, then "=".
func parse(body string) []Def {
	var defs []Def
	for _, f := range strings.Fields(body) {
		id, value, ok := strings.Cut(f, "=")
		if ok && isID(id) {
			defs = append(defs, Def{id, value})
		} else if len(defs) > 0 {
			defs[len(defs)-1].Value += " " + f
		}
	}
	return defs
}

func isID(s string) bool {
	for _, sg := range sigil {
		if strings.HasPrefix(s, sg) && len(s) > len(sg) {
			return true
		}
	}
	return false
}
