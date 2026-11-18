package dboptimizer

import (
	"context"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// TestDispatch pins which handler each dialect reaches (and the fallback).
func TestDispatch(t *testing.T) {
	text := ` Limit  (cost=1.00..2.00 rows=10 width=8) (actual time=100.000..100.500 rows=10 loops=1)
   ->  Seq Scan on big b  (cost=0.00..9000.00 rows=500000 width=8) (actual time=0.010..99.000 rows=500000 loops=1)
 Execution Time: 100.9 ms
1) "s:1"
2) "s:22"
3) "s:333"
[ { _id: ObjectId('0123456789abcdef01234567'), name: 'a', n: 1, } ]
 id | name
----+------
  1 | a
  2 | b
(2 rows)
Applying migration 001_init ... done
`
	o := &Optimizer{cfg: &Config{Limits: table.Limits{MaxRows: 30, MaxCell: 60}, PlanMinShare: 0.02}}
	opts := o.cfg.Options(nil)
	want := map[string]func([]string) []string{
		"explain": o.plan,
		"redis":   redis,
		"mongosh": func([]string) []string { return mongo(text, opts) },
		"migrate": migrations,
		"unknown": func(lines []string) []string { return results(lines, opts) },
	}
	for dialect, handler := range want {
		t.Run(dialect, func(t *testing.T) {
			p := payload.New(payload.CmdOutput, text)
			p.Blocks[0].Type = "db/" + dialect
			if err := o.Optimize(context.Background(), p); err != nil {
				t.Fatal(err)
			}
			exp := compress.Join(handler(compress.CleanLines(text)))
			if p.Blocks[0].Text != exp {
				t.Errorf("got:\n%s\nwant:\n%s", p.Blocks[0].Text, exp)
			}
		})
	}
}
