package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
)

func gainFlags(f gainViewFlags) gainViewFlags {
	f.sort = "saved"
	f.top = 10
	return f
}

func TestGainViewSections(t *testing.T) {
	tests := []struct {
		name  string
		flags gainViewFlags
		on    []string // sections shown; nil = all
		err   string
	}{
		{name: "default", on: minimalSections},
		{name: "full", flags: gainViewFlags{full: true}},
		{name: "only", flags: gainViewFlags{show: "agent, model"}, on: []string{"agent", "model"}},
		{name: "hide", flags: gainViewFlags{full: true, hide: "estimates,dialect"}},
		{name: "hide default", flags: gainViewFlags{hide: "agent"}, on: []string{"trend", "command", "failing"}},
		{name: "only then hide", flags: gainViewFlags{show: "agent,model", hide: "model"}, on: []string{"agent"}},
		{name: "full and show", flags: gainViewFlags{full: true, show: "cost"}, on: []string{"cost"}},
		{name: "unknown", flags: gainViewFlags{show: "agnt"}, err: `unknown section "agnt"`},
		{name: "bad sort", flags: gainViewFlags{sort: "x"}, err: "unknown -sort"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := gainFlags(tt.flags)
			if tt.flags.sort != "" {
				f.sort = tt.flags.sort
			}
			v, err := f.build(true)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("err = %v, want %q", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, n := range overviewSectionNames() {
				want := tt.on == nil || slices.Contains(tt.on, n)
				if tt.name == "hide" && (n == "estimates" || n == "dialect") {
					want = false
				}
				if v.show(n) != want {
					t.Errorf("show(%s) = %v, want %v", n, v.show(n), want)
				}
			}
		})
	}
	if _, err := gainFlags(gainViewFlags{full: true}).build(false); err == nil {
		t.Error("-full with -by should fail")
	}
}

func TestGainViewRows(t *testing.T) {
	var rows []mcptool.StatsJSON
	for i, g := range []string{"b", "a", "d", "c"} {
		rows = append(rows, mcptool.StatsJSON{Group: g, Runs: 4 - i, RawTokens: 100 * (i + 1), Saved: 10 * (i + 1) * (i%2 + 1), SavedPct: float64(i)})
	}
	groups := func(rs []mcptool.StatsJSON) string {
		var s []string
		for _, r := range rs {
			s = append(s, r.Group)
		}
		return strings.Join(s, "")
	}
	tests := []struct {
		view gainView
		want string
		more int
	}{
		{gainView{sort: "saved"}, "cadb", 0},
		{gainView{sort: "name"}, "abcd", 0},
		{gainView{sort: "runs"}, "badc", 0},
		{gainView{sort: "in"}, "cdab", 0},
		{gainView{sort: "pct"}, "cdab", 0},
		{gainView{sort: "name", top: 3}, "abc", 1},
		{gainView{sort: "name", minTokens: 250}, "cd", 0},
		{gainView{sort: "name", minTokens: 250, top: 1}, "c", 1},
	}
	for _, tt := range tests {
		shown, more := tt.view.rows(rows)
		if groups(shown) != tt.want || more != tt.more {
			t.Errorf("%+v: %s +%d, want %s +%d", tt.view, groups(shown), more, tt.want, tt.more)
		}
	}
	if groups(rows) != "badc" {
		t.Error("rows must not reorder the caller's slice")
	}
}

func TestRenderOverviewSections(t *testing.T) {
	l, _ := testDashboard(t)
	for i := range 15 {
		record(t, l, "s1", fmt.Sprintf("cmd%02d", i))
	}
	o, err := mcptool.BuildOverview(t.Context(), l, component.StatsQuery{})
	if err != nil {
		t.Fatal(err)
	}
	render := func(f gainViewFlags) string {
		v, err := gainFlags(f).build(true)
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		painter{}.renderOverview(&b, o, 0, nil, nil, v)
		return b.String()
	}
	out := render(gainViewFlags{})
	for _, want := range []string{"gascoigne savings", "LAST 7 DAYS", "COMMANDS", "… 5 more (-top 0 shows all)"} {
		if !strings.Contains(out, want) {
			t.Errorf("default overview lacks %q:\n%s", want, out)
		}
	}
	for _, not := range []string{"OUTCOMES", "OPTIMIZERS", "ESTIMATES", "cmd14"} {
		if strings.Contains(out, not) {
			t.Errorf("default overview has %q:\n%s", not, out)
		}
	}
	if full := render(gainViewFlags{full: true}); !strings.Contains(full, "OUTCOMES") || !strings.Contains(full, "ESTIMATES") {
		t.Errorf("-full:\n%s", full)
	}
	if only := render(gainViewFlags{show: "outcome"}); strings.Contains(only, "COMMANDS") || !strings.Contains(only, "OUTCOMES") {
		t.Errorf("-show outcome:\n%s", only)
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	for d, want := range map[time.Duration]string{
		10 * time.Second: "just now", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago",
		50 * time.Hour: "2d ago", 10 * 24 * time.Hour: "2026-10-06",
	} {
		if got := ago(now.Add(-d), now); got != want {
			t.Errorf("ago(-%v) = %q, want %q", d, got, want)
		}
	}
}

func TestGainCSV(t *testing.T) {
	r := mcptool.Report(component.StatsQuery{By: "command"}, []component.Stat{
		{Group: `git "log"`, Runs: 2, Optimized: 1, RawTokens: 2000, OutTokens: 500},
		{Group: "ls", Runs: 1, Failed: 1, RawTokens: 100, OutTokens: 100},
	})
	var b strings.Builder
	if err := statsCSV(&b, r); err != nil {
		t.Fatal(err)
	}
	want := "command,runs,optimized,failed,in_tokens,out_tokens,saved_tokens,saved_pct\n" +
		"\"git \"\"log\"\"\",2,1,0,2000,500,1500,75\n" +
		"ls,1,0,1,100,100,0,0\n" +
		"total,3,1,1,2100,600,1500,71.4\n"
	if b.String() != want {
		t.Errorf("csv:\n%s\nwant:\n%s", b.String(), want)
	}
}
