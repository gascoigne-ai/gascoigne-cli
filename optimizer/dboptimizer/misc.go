package dboptimizer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	redisItem = lazyre.New(`^\s*\d+\) "?(.*?)"?$`)
	digits    = lazyre.New(`\d+|[0-9a-f]{8,}`)
)

// redis folds numbered reply lists (KEYS, SCAN, SMEMBERS) by pattern:
// "session:* ×812 (session:1, session:2…)".
func redis(lines []string) []string {
	type group struct {
		n        int
		examples []string
	}
	groups := map[string]*group{}
	var order, other []string
	for _, l := range lines {
		m := redisItem.FindStringSubmatch(l)
		if m == nil {
			other = append(other, l)
			continue
		}
		pat := digits.ReplaceAllString(m[1], "*")
		g, ok := groups[pat]
		if !ok {
			g = &group{}
			groups[pat] = g
			order = append(order, pat)
		}
		g.n++
		if len(g.examples) < 2 {
			g.examples = append(g.examples, m[1])
		}
	}
	if len(order) == 0 {
		return compress.DedupeRuns(lines)
	}
	out := other
	for _, p := range order {
		g := groups[p]
		if g.n == 1 {
			out = append(out, g.examples[0])
			continue
		}
		out = append(out, fmt.Sprintf("%s ×%d (%s…)", p, g.n, strings.Join(g.examples, ", ")))
	}
	return out
}

var (
	mongoWrap = lazyre.New(`(ObjectId|ISODate|NumberLong|NumberInt|NumberDecimal|UUID|Timestamp|BinData)\(\s*(?:'([^']*)'|"([^"]*)"|([^)]*))\s*\)`)
	mongoKey  = lazyre.New(`([{,]\s*)([A-Za-z_$][\w$]*)\s*:`)
	mongoStr  = lazyre.New(`'((?:[^'\\]|\\.)*)'`)
	trailing  = lazyre.New(`,(\s*[}\]])`)
)

// mongo turns mongosh output (JS-like documents) into JSON and samples the
// documents; output that does not convert stays as is.
func mongo(text string, o table.Options) []string {
	js := mongoWrap.ReplaceAllString(text, `"$2$3$4"`)
	js = mongoStr.ReplaceAllString(js, `"$1"`)
	js = mongoKey.ReplaceAllString(js, `$1"$2":`)
	js = trailing.ReplaceAllString(js, "$1")
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(js)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return compress.Lines(text)
	}
	docs, ok := v.([]any)
	if !ok {
		docs = []any{v}
	}
	opts := o.Rows
	opts.MaxItems = o.MaxRows
	var out []string
	for _, d := range sample.Sample(docs, opts) {
		b, _ := json.Marshal(d)
		out = append(out, string(b))
	}
	return out
}

var migrateNoise = lazyre.New(`^INFO  \[alembic\.runtime\.migration\] (Context impl|Will assume)|^Environment variables loaded|^Prisma schema loaded|^Datasource "|^\s*$|^Loaded Prisma config`)

// migrations drop tool chatter; applied steps, pending ones and errors stay.
func migrations(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !migrateNoise.MatchString(l) {
			out = append(out, l)
		}
	}
	return compress.DedupeRuns(out)
}
