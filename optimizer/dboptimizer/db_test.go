package dboptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

func TestPlanKeepsSlowNodes(t *testing.T) {
	in := ` Limit  (cost=1.00..2.00 rows=10 width=8) (actual time=100.000..100.500 rows=10 loops=1)
   ->  Seq Scan on big b  (cost=0.00..9000.00 rows=500000 width=8) (actual time=0.010..99.000 rows=500000 loops=1)
         Filter: (x > 1)
         Buffers: shared hit=10
   ->  Index Scan using t_pkey on tiny t  (cost=0.28..1.00 rows=1 width=8) (actual time=0.001..0.002 rows=1 loops=1)
 Execution Time: 100.9 ms
`
	o := &Optimizer{cfg: &Config{PlanMinShare: 0.02}}
	got := compress.Join(o.plan(compress.Lines(in)))
	for _, want := range []string{"Seq Scan on big b (time=99.000ms rows=500000 est=500000 loops=1)", "Filter: (x > 1)", "1 cheap plan node omitted", "Execution Time"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Buffers") || strings.Contains(got, "tiny") {
		t.Errorf("kept noise:\n%s", got)
	}
}

func TestRedisFolds(t *testing.T) {
	got := compress.Join(redis(compress.Lines("1) \"s:1\"\n2) \"s:22\"\n3) \"s:333\"\n4) \"cfg\"\n")))
	if got != "s:* ×3 (s:1, s:22…)\ncfg\n" {
		t.Fatalf("got %q", got)
	}
}

func TestMongoToJSON(t *testing.T) {
	in := "[ { _id: ObjectId('0123456789abcdef01234567'), name: 'a', at: ISODate('2026-01-01T00:00:00Z'), n: 1, } ]"
	got := mongo(in, tableOpts())
	if len(got) != 1 || got[0] != `{"_id":"0123456789abcdef01234567","at":"2026-01-01T00:00:00Z","n":1,"name":"a"}` {
		t.Fatalf("got %q", got)
	}
}

func tableOpts() table.Options { return table.Options{MaxRows: 30} }
