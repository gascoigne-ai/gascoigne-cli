// Package dboptimizer is the db family engine: query results (psql, mysql,
// sqlite3) as compact sampled tables, EXPLAIN plans pruned to costly nodes,
// redis key listings folded by pattern, mongosh documents sampled as JSON,
// migration tool chatter dropped.
package dboptimizer

import (
	"context"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	opts := table.Options{MaxRows: o.cfg.MaxRows, MaxCell: o.cfg.MaxCell,
		Rows: sample.Options{HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2, RelevanceK: 3, Query: sample.Terms(p.Attrs[payload.AttrQuery])}}
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyDB {
			continue
		}
		lines := compress.Lines(compress.StripANSI(b.Text))
		var out []string
		switch detect.Dialect(b.Type) {
		case "explain":
			out = o.plan(lines)
		case "redis":
			out = redis(lines)
		case "mongosh":
			out = mongo(b.Text, opts)
		case "migrate":
			out = migrations(lines)
		default:
			out = results(lines, opts)
		}
		p.Blocks[i].Text = compress.Join(out)
	}
	return nil
}

// footer ends a result set: "(3 rows)", "3 rows in set (0.00 sec)", "Empty set".
var footer = lazyre.New(`^\(\d+ rows?\)$|^\d+ rows? in set|^Empty set|^Query OK`)

// results renders each result set of the output as a compact table.
func results(lines []string, o table.Options) []string {
	var out, chunk []string
	flush := func() {
		if len(chunk) == 0 {
			return
		}
		if t, ok := table.Parse(chunk); ok {
			out = append(out, table.Render(t, o)...)
		} else {
			out = append(out, chunk...)
		}
		chunk = nil
	}
	for _, l := range lines {
		chunk = append(chunk, l)
		if footer.MatchString(strings.TrimSpace(l)) {
			flush()
		}
	}
	flush()
	return compress.SqueezeBlank(out)
}
