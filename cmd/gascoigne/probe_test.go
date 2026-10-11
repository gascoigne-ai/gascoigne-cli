package main

import (
	"testing"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

func TestProbeRuns(t *testing.T) {
	l, _ := testDashboard(t)
	ctx := t.Context()
	runs := []struct{ route, orig, out string }{
		{"default", "a.go:3: undefined: x\nb.go:9: cannot use y\n", "a.go:3: undefined: x\n"},
		// A fold points at an earlier result holding everything: not a loss.
		{"fold", "c.go:1: undefined: z\n", "[↑ L1 = L1 of earlier Read]\n"},
	}
	for _, r := range runs {
		p := &payload.Payload{Blocks: []payload.Block{{Kind: payload.CmdOutput, Text: r.orig}}}
		id, err := l.SaveRaw(ctx, "cmd:"+r.route, p)
		if err != nil {
			t.Fatal(err)
		}
		ev := component.Event{TS: time.Now(), Command: "go build", Route: r.route, RawID: id, Output: r.out,
			Outcome: component.Optimized, RawTokens: 20, OutTokens: 8,
			Steps: []component.Step{{Optimizer: "go", Family: "build", RawTokens: 20, OutTokens: 8}}}
		if err := l.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}

	got, err := probeRuns(ctx, l, component.StatsQuery{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d runs, want 1 (fold skipped)", len(got))
	}
	if r := got[0].Report.Errors; r.Total != 2 || len(r.Lost) != 1 || r.Lost[0] != "b.go:9: cannot use y" {
		t.Errorf("errors %+v", r)
	}
	fam := byFamily(got)
	if len(fam) != 1 || fam[0].Family != "build" || fam[0].Errors != (kept{Total: 2, Kept: 1}) {
		t.Errorf("by family %+v", fam)
	}
}
