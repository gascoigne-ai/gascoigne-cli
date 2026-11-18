// Package dboptimizer is the db family engine: query results (psql, mysql,
// sqlite3) as compact sampled tables, EXPLAIN plans pruned to costly nodes,
// redis key listings folded by pattern, mongosh documents sampled as JSON,
// migration tool chatter dropped.
package dboptimizer

import (
	"context"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// handlers compress one dialect; others are result sets.
var handlers = map[string]func(opt *Optimizer, block payload.Block, lines []string, opts table.Options) []string{
	"explain": func(opt *Optimizer, _ payload.Block, lines []string, _ table.Options) []string {
		return opt.plan(lines)
	},
	"redis": func(_ *Optimizer, _ payload.Block, lines []string, _ table.Options) []string { return redis(lines) },
	"mongosh": func(_ *Optimizer, block payload.Block, _ []string, opts table.Options) []string {
		return mongo(block.Text, opts)
	},
	"migrate": func(_ *Optimizer, _ payload.Block, lines []string, _ table.Options) []string {
		return migrations(lines)
	},
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	opts := o.cfg.Options(sample.Terms(data.Attrs[payload.AttrQuery]))
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		lines := compress.CleanLines(block.Text)
		handle, ok := handlers[detect.Dialect(block.Type)]
		if !ok {
			handle = func(_ *Optimizer, _ payload.Block, lines []string, opts table.Options) []string {
				return results(lines, opts)
			}
		}
		out := handle(o, block, lines, opts)
		return compress.Join(out), nil
	}, detect.FamilyDB)
}

// footer ends a result set: "(3 rows)", "3 rows in set (0.00 sec)", "Empty set".
var footer = lazyre.New(`^\(\d+ rows?\)$|^\d+ rows? in set|^Empty set|^Query OK`)

// results renders each result set of the output as a compact table.
func results(lines []string, options table.Options) []string {
	var out, chunk []string
	flush := func() {
		if len(chunk) == 0 {
			return
		}
		if rendered, ok := table.Compress(chunk, options); ok {
			out = append(out, rendered...)
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
