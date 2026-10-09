package logoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(typ, in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}}
	(&Optimizer{cfg: &Config{Context: 1, DropDebug: true, MaxLines: 300}}).Optimize(nil, p)
	return p.Blocks[0].Text
}

func TestLog(t *testing.T) {
	in := `10:00:01 INFO GET /a status=200 dur=3
10:00:02 DEBUG cache hit key=1
10:00:03 INFO GET /a status=200 dur=9
10:00:04 DEBUG cache hit key=2
10:00:05 INFO before failure
10:00:06 ERROR db: deadlock detected
    at repo.commit (repo.ts:1)
    at svc.place (svc.ts:2)
10:00:07 INFO after failure
10:00:08 INFO GET /a status=200 dur=1
10:00:09 WARN retry attempt=1
10:00:10 WARN retry attempt=2
10:00:11 WARN retry attempt=3
`
	got := run(detect.Log, in)
	want := `10:00:01 INFO GET /a status=200 dur=3 (×2)
… 3 lines omitted
10:00:05 INFO before failure
10:00:06 ERROR db: deadlock detected
    at repo.commit (repo.ts:1)
    at svc.place (svc.ts:2)
10:00:07 INFO after failure
10:00:08 INFO GET /a status=200 dur=1
10:00:09 WARN retry attempt=1 (×3)
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTextOnlyLosesRepeats(t *testing.T) {
	in := "Step 1\nStep 2\nsame\nsame\nsame\n\n\n\nend\n"
	if got, want := run(detect.Text, in), "Step 1\nStep 2\nsame (×3)\n\nend\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestLimitKeepsErrors(t *testing.T) {
	var b strings.Builder
	for i := range 2000 {
		b.WriteString("INFO unique line " + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + strings.Repeat("y", i%11) + "\n")
		if i == 1000 {
			b.WriteString("ERROR the one that matters\n")
		}
	}
	p := &payload.Payload{Blocks: []payload.Block{{Text: b.String(), Type: detect.Log}}}
	(&Optimizer{cfg: &Config{Context: 0, DropDebug: true, MaxLines: 40}}).Optimize(nil, p)
	got := p.Blocks[0].Text
	if !strings.Contains(got, "ERROR the one that matters") || strings.Count(got, "\n") > 45 {
		t.Fatalf("got %d lines:\n%s", strings.Count(got, "\n"), got)
	}
}
