package logoptimizer

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func run(typ, in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}}
	(&Optimizer{cfg: &Config{Context: 1, DropDebug: true, MaxLines: 300, Passes: defaultPasses}}).Optimize(nil, p)
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

func TestRepeatGapSaysSo(t *testing.T) {
	trace := "java.lang.IllegalStateException: pool shut down\n" +
		"\tat org.apache.http.util.Asserts.check(Asserts.java:34)\n" +
		"\tat com.example.Gateway.charge(Gateway.java:118)\n" +
		"\tat com.example.Worker.run(Worker.java:41)\n"
	in := "10:00 INFO start\n" + trace + "10:01 INFO next\n" + trace + "10:04 INFO end\n"
	got := run(detect.Log, in)
	if !strings.Contains(got, "10:01 INFO next\n… 4 repeats of lines above\n") {
		t.Fatalf("repeated trace gap:\n%s", got)
	}
	in = "10:00 INFO start\n" + trace + "10:01 INFO next\n" + trace + "10:02 DEBUG tick\n10:04 INFO end\n"
	if got := run(detect.Log, in); !strings.Contains(got, "10:01 INFO next\n… 5 lines omitted\n10:04") {
		t.Fatalf("mixed gap:\n%s", got)
	}
}

func TestTextOnlyLosesRepeats(t *testing.T) {
	in := "Step 1\nStep 2\nsame\nsame\nsame\n\n\n\nend\n"
	if got, want := run(detect.Text, in), "Step 1\nStep 2\nsame (×3)\n\nend\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Unknown text with many lines of one shape folds to the first plus a count;
// prose and short text stay whole.
func TestTextFoldsRepeatedTemplates(t *testing.T) {
	var rep, rows, prose strings.Builder
	for i := range 30 {
		fmt.Fprintf(&rep, "2026-01-02T03:04:%02dZ connect to 10.0.0.1 port 80 ok\n", i)
		fmt.Fprintf(&rows, "INSERT INTO t VALUES (%d, %d);\n", i, i*7)
		fmt.Fprintf(&prose, "Sentence %s about %s.\n", strings.Repeat("w", i+1), strings.Repeat("z", 30-i))
	}
	tests := []struct {
		name, in string
		folds    bool
	}{
		{"repeated", rep.String(), true},
		{"prose", prose.String(), false},
		{"data rows", rows.String(), false},
		{"short", "[1] a ok\n[2] a ok\n[3] a ok\n[4] a ok\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(detect.Text, tt.in)
			if folded := strings.Contains(got, "×30"); folded != tt.folds {
				t.Fatalf("folded=%v, want %v:\n%s", folded, tt.folds, got)
			}
			if !tt.folds && got != tt.in {
				t.Fatalf("changed:\n%s", got)
			}
		})
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

func TestEntriesCloudEvents(t *testing.T) {
	got, ok := entries(`{"events": [{"timestamp": 1791615600000, "message": "START\n", "logStreamName": "s"}], "nextToken": "x"}`)
	if !ok || len(got) != 1 || got[0] != "2026-10-10T07:00:00.000Z START" {
		t.Fatalf("got %q %v", got, ok)
	}
	got, ok = entries("severity: ERROR\ntimestamp: '2026-10-10T07:00:00Z'\nhttpRequest: {requestMethod: GET, requestUrl: /x, status: 500}\n---\nseverity: INFO\njsonPayload: {message: hi}\n")
	if !ok || got[0] != "2026-10-10T07:00:00Z ERROR GET /x 500" || got[1] != "INFO hi" {
		t.Fatalf("got %q %v", got, ok)
	}
	got, ok = entries(`{"time": "t1", "level": "warn", "msg": "slow", "path": "/a", "ms": 12}` + "\n")
	if !ok || got[0] != "t1 WARN slow ms=12 path=/a" {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := entries("plain text\nlines\n"); ok {
		t.Fatal("plain text taken as events")
	}
}

func TestAccessSummary(t *testing.T) {
	var lines []string
	for i := range 20 {
		lines = append(lines, fmt.Sprintf(`10.0.0.%d - - [10/Oct/2026:07:00:%02d +0000] "GET /api/items/%d?x=1 HTTP/1.1" 200 512 "-" "curl/8"`, i%3, i, i))
	}
	lines = append(lines, `10.0.0.9 - - [10/Oct/2026:07:01:00 +0000] "POST /api/orders HTTP/1.1" 502 0 "-" "curl/8"`)
	o := &Optimizer{cfg: &Config{Context: 2, MaxLines: 300}}
	got := compress.Join(o.access(lines))
	for _, want := range []string{"21 requests 10/Oct/2026:07:00:00 +0000 – 10/Oct/2026:07:01:00 +0000: 2xx 20, 5xx 1",
		"GET /api/items/:id ×20 (200 20)", "POST /api/orders ×1 (502 1)", "server errors:\n" + lines[20]} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestUnitPairs(t *testing.T) {
	in := []string{
		"Oct 10 00:05:44 host systemd[1]: Starting CUPS Scheduler...",
		"Oct 10 00:05:44 host systemd[1]: Starting Permit User Sessions...",
		"Oct 10 00:05:44 host systemd[1]: Finished Permit User Sessions.",
		"Oct 10 00:05:45 host systemd[1]: Starting WPA supplicant...",
		"Oct 10 00:05:45 host systemd[1]: Started WPA supplicant.",
		"Oct 10 00:05:46 host systemd[1234]: Started WPA supplicant.",
		"Oct 10 00:06:00 host systemd[1]: Stopping Disk Manager...",
		"Oct 10 00:06:01 host systemd[1]: Stopped Disk Manager.",
	}
	want := []string{
		"Oct 10 00:05:44 host systemd[1]: Starting CUPS Scheduler...", // never started: stays
		"Oct 10 00:05:44 host systemd[1]: Finished Permit User Sessions.",
		"Oct 10 00:05:45 host systemd[1]: Started WPA supplicant.",
		"Oct 10 00:05:46 host systemd[1234]: Started WPA supplicant.", // another process: no pair
		"Oct 10 00:06:01 host systemd[1]: Stopped Disk Manager.",
	}
	if got := unitPairs(in); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s", strings.Join(got, "\n"))
	}
}

func TestNumericOutlierKept(t *testing.T) {
	tests := []struct {
		name string
		durs []int
		kept string // line that must stay, "" when none stands out
	}{
		{"slow request", []int{21, 34, 25, 40, 9814, 33, 28, 22, 37, 30}, "batch 4 synced in 9814ms"},
		{"steady", []int{21, 34, 25, 40, 61, 33, 28, 22, 37, 30}, ""},
		{"too few to tell", []int{21, 9814, 25}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			for i, d := range tt.durs {
				fmt.Fprintf(&b, "10:00:%02d INFO batch %d synced in %dms\n", i, i, d)
			}
			got := run(detect.Log, b.String())
			if tt.kept != "" && !strings.Contains(got, tt.kept) {
				t.Errorf("outlier %q folded:\n%s", tt.kept, got)
			}
			if lines := len(compress.Lines(got)); tt.kept == "" && lines > 2 {
				t.Errorf("want one template line and a gap, got:\n%s", got)
			}
		})
	}
}

func TestShortLevelValueIsNotDebug(t *testing.T) {
	for _, l := range []string{"level=E msg=boom", "level=D msg=boom", "level=bug msg=boom"} {
		if isDebug(l) {
			t.Errorf("isDebug(%q) = true", l)
		}
	}
	for _, l := range []string{"10:00 DEBUG x", "level=debug msg=x", "level=TRACE msg=x"} {
		if !isDebug(l) {
			t.Errorf("isDebug(%q) = false", l)
		}
	}
}

func TestFieldsKeepsDottedTopLevelKeys(t *testing.T) {
	got := fields(map[string]any{"service.name": "api", "msg": "hi", "n": float64(3)})
	if got != "n=3 service.name=api" {
		t.Fatalf("got %q", got)
	}
}

// Lines that differ by a host name, not only by numbers, stayed one line each
// under the exact key; the drain miner folds them into one example with a count.
func TestDrainMinerFoldsVariableWords(t *testing.T) {
	var lines []string
	for _, h := range []string{"alpha", "beta", "gamma", "delta", "omega", "sigma"} {
		lines = append(lines, "10:00:01 INFO connection to host "+h+" established ok")
	}
	lines = append(lines, "10:00:02 ERROR connection to host zeta lost", "10:00:03 ERROR connection to host eta lost")
	tests := []struct {
		name  string
		miner string
		want  string
	}{
		{"regex keeps each host", "", "host sigma established"},
		{"drain keeps three hosts, names the rest", "drain", "(×3; also omega, sigma)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &Optimizer{cfg: &Config{Context: 0, DropDebug: true, MaxLines: 300, Miner: tt.miner,
				DrainDepth: 4, DrainSimilarity: 0.6, DrainMaxChildren: 100, DrainVariants: 3, DrainMaxVars: 2}}
			p := &payload.Payload{Blocks: []payload.Block{{Text: compress.Join(lines), Type: detect.Log}}}
			o.Optimize(nil, p)
			got := p.Blocks[0].Text
			if !strings.Contains(got, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, got)
			}
			for _, e := range []string{"host zeta lost", "host eta lost"} {
				if !strings.Contains(got, e) {
					t.Errorf("error line %q lost:\n%s", e, got)
				}
			}
		})
	}
}

func TestConfigValidateMiner(t *testing.T) {
	ok := Config{Context: 2, MaxLines: 300, DrainDepth: 4, DrainSimilarity: 0.6, DrainMaxChildren: 100, DrainVariants: 3}
	tests := []struct {
		name   string
		mutate func(*Config)
		bad    bool
	}{
		{"default regex", func(c *Config) { c.Miner = "regex" }, false},
		{"empty is regex", func(c *Config) {}, false},
		{"drain", func(c *Config) { c.Miner = "drain" }, false},
		{"unknown miner", func(c *Config) { c.Miner = "lcs" }, true},
		{"drain depth too small", func(c *Config) { c.Miner = "drain"; c.DrainDepth = 2 }, true},
		{"drain similarity zero", func(c *Config) { c.Miner = "drain"; c.DrainSimilarity = 0 }, true},
		{"drain similarity above one", func(c *Config) { c.Miner = "drain"; c.DrainSimilarity = 1.5 }, true},
		{"drain no variants", func(c *Config) { c.Miner = "drain"; c.DrainVariants = 0 }, true},
		{"drain no children", func(c *Config) { c.Miner = "drain"; c.DrainMaxChildren = 0 }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ok
			tt.mutate(&c)
			if err := c.Validate(); (err != nil) != tt.bad {
				t.Fatalf("Validate() = %v, want error: %v", err, tt.bad)
			}
		})
	}
}

// appLog50000 is the scale suite's app-log case: one template, 50k lines, one error.
func appLog50000() string {
	var b strings.Builder
	for i := 1; i <= 50000; i++ {
		fmt.Fprintf(&b, "2026-10-10T12:%02d:%02d.%03dZ INFO worker-%d request id=%d path=/api/v1/items status=200 took=%dms\n",
			i/60%60, i%60, i%1000, i%8, i, i%90)
	}
	b.WriteString("2026-10-10T13:00:00.000Z ERROR worker-3 request id=99999 failed: connection refused\n")
	return b.String()
}

func BenchmarkOptimizeAppLog(b *testing.B) {
	in := appLog50000()
	for _, miner := range []string{minerRegex, minerDrain} {
		b.Run(miner, func(b *testing.B) {
			o := &Optimizer{cfg: &Config{Context: 2, DropDebug: true, MaxLines: 300, Miner: miner,
				DrainDepth: 4, DrainSimilarity: 0.6, DrainMaxChildren: 100, DrainMaxVars: 2, DrainVariants: 3}}
			for b.Loop() {
				p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.Log}}}
				o.Optimize(nil, p)
			}
		})
	}
}

// Over the line cap, head, tail and errors were all that survived: a template
// seen once in the middle went with the noise. Rarity spends the spare budget
// on the templates that stand for the fewest lines.
func TestLimitRarityKeepsRareTemplates(t *testing.T) {
	var es []entry
	for i := range 200 {
		es = append(es, entry{text: fmt.Sprintf("common template %d (×500)", i), n: 500})
	}
	es[90] = entry{text: "seen only once", n: 1}
	es[120] = entry{text: "seen twice", n: 2}
	tests := []struct {
		name   string
		rarity bool
		want   []string
		not    []string
	}{
		{"off drops the middle", false, nil, []string{"seen only once", "seen twice"}},
		{"on keeps the rare ones", true, []string{"seen only once", "seen twice"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := &Optimizer{cfg: &Config{MaxLines: 40, Rarity: tt.rarity}}
			got := strings.Join(texts(o.limit(es)), "\n")
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
			for _, w := range tt.not {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in:\n%s", w, got)
				}
			}
			if n := len(o.limit(es)); n > 2*o.cfg.MaxLines {
				t.Errorf("%d entries, cap is %d", n, o.cfg.MaxLines)
			}
		})
	}
}

func TestLimitRarityAddsABoundedShare(t *testing.T) {
	var es []entry
	for i := range 200 {
		es = append(es, entry{text: fmt.Sprintf("line %d", i), n: 1 + i%3})
	}
	o := &Optimizer{cfg: &Config{MaxLines: 40, Rarity: true}}
	got := o.limit(es)
	lines := 0
	for _, e := range got {
		if !strings.HasPrefix(e.text, "… ") {
			lines++
		}
	}
	// head 10 + tail 10 + MaxLines/rarityShare (5) rarest
	if want := 10 + 10 + 40/rarityShare; lines != want {
		t.Fatalf("kept %d lines, want %d", lines, want)
	}
}

// Numbers are masked so lines differing in them fold, but a status or exit
// code is a category: one `status=503` among 29 `status=200` was folded into
// "(×30)" and vanished from the output.
func TestCategoricalCodesDoNotFold(t *testing.T) {
	line := func(i int, status string) string {
		return fmt.Sprintf("2026-10-10T12:00:%02dZ INFO GET /api/items %s took=%dms\n", i, status, 10+i%3)
	}
	tests := []struct {
		name   string
		status func(i int) string
		want   []string
	}{
		{"status key", func(i int) string {
			if i == 17 {
				return "status=503"
			}
			return "status=200"
		}, []string{"status=503", "status=200"}},
		{"colon form", func(i int) string {
			if i == 17 {
				return "status: 503"
			}
			return "status: 200"
		}, []string{"status: 503", "status: 200"}},
		{"exit code words", func(i int) string {
			if i == 17 {
				return "exit code 137"
			}
			return "exit code 0"
		}, []string{"exit code 137", "exit code 0"}},
		{"plain counters still fold", func(i int) string { return fmt.Sprintf("retries=%d", i) }, []string{"(×30)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var in strings.Builder
			for i := range 30 {
				in.WriteString(line(i, tt.status(i)))
			}
			got := run(detect.Log, in.String())
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in:\n%s", w, got)
				}
			}
		})
	}
}

// Kept lines that differ only in a code sit next to each other when the lines
// between them are dropped; merging runs must not fold them back together.
func TestCategoricalCodesNotMergedAsRun(t *testing.T) {
	in := "INFO GET /a status=200 took=5ms\nINFO GET /a status=503 took=5ms\n"
	got := run(detect.Log, strings.Repeat(in, 10))
	if !strings.Contains(got, "status=503") || !strings.Contains(got, "status=200") {
		t.Fatalf("a code vanished:\n%s", got)
	}
}

func TestProtectCodes(t *testing.T) {
	tests := []struct{ in, want string }{
		{"status=200 took=10ms", "status=CAA took=10ms"},
		{"Status: 404 not found", "Status: EAE not found"},
		{"exit status 1", "exit status B"},
		{"exit code 137 after 5s", "exit code BDH after 5s"},
		{"request id=42 retries=3", "request id=42 retries=3"},
		{"status=2000", "status=2000"},
		{"xstatus=200", "xstatus=200"},
		{"status=200abc", "status=200abc"},
		{"status=", "status="},
		{"code", "code"},
		{"", ""},
		{"rc=0 errno=2", "rc=A errno=C"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := protectCodes(tt.in); got != tt.want {
				t.Errorf("protectCodes(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// A latency that is 4x its usual value is far outside a steady spread yet
// below the 10x-median rule, so it folded into the ×N of its template.
func TestMADOutlierKept(t *testing.T) {
	steady := []int{101, 98, 103, 99, 100, 102, 97, 100, 101, 99, 100, 98, 102, 100}
	spike := append(slices.Clone(steady), 400)
	same := slices.Repeat([]int{10}, 14)
	tests := []struct {
		name     string
		outliers string
		durs     []int
		kept     string // "" when nothing may stand out
	}{
		{"ratio misses a 4x spike", "ratio", spike, ""},
		{"mad keeps a 4x spike", "mad", spike, "synced in 400ms"},
		{"mad ignores a steady spread", "mad", steady, ""},
		{"mad ignores a tiny step over identical values", "mad", append(slices.Clone(same), 11), ""},
		{"mad keeps a clear step over identical values", "mad", append(slices.Clone(same), 40), "synced in 40ms"},
		{"mad still keeps a 100x spike", "mad", append(slices.Clone(steady), 9814), "synced in 9814ms"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b strings.Builder
			for i, d := range tt.durs {
				fmt.Fprintf(&b, "10:00:%02d INFO batch %d synced in %dms\n", i, i, d)
			}
			p := &payload.Payload{Blocks: []payload.Block{{Text: b.String(), Type: detect.Log}}}
			(&Optimizer{cfg: &Config{Context: 0, DropDebug: true, MaxLines: 300, Outliers: tt.outliers}}).Optimize(nil, p)
			got := p.Blocks[0].Text
			if tt.kept != "" && !strings.Contains(got, tt.kept) {
				t.Errorf("outlier %q folded:\n%s", tt.kept, got)
			}
			if tt.kept == "" && len(compress.Lines(got)) > 2 {
				t.Errorf("want one template line and a gap, got:\n%s", got)
			}
		})
	}
}

// A template that shows up only late in a long log is the first line of its
// kind, and the first line of every template always stays: no separate
// novelty detector is needed. This pins that.
func TestLateTemplateStays(t *testing.T) {
	var b strings.Builder
	for i := range 400 {
		fmt.Fprintf(&b, "10:%02d:%02d INFO heartbeat ok seq=%d\n", i/60, i%60, i)
	}
	b.WriteString("10:59:59 INFO certificate for api.internal rotated\n")
	for i := range 20 {
		fmt.Fprintf(&b, "11:00:%02d INFO heartbeat ok seq=%d\n", i, 400+i)
	}
	got := run(detect.Log, b.String())
	if !strings.Contains(got, "certificate for api.internal rotated") {
		t.Fatalf("late template folded away:\n%s", got)
	}
	if !strings.Contains(got, "(×420)") {
		t.Fatalf("heartbeats not counted:\n%s", got)
	}
}

// ×N says how often a line repeated, not when. Repeats packed into a short
// stretch of a long log (a retry storm) are worth a pointer to those lines.
func TestBurstNamesTheStretch(t *testing.T) {
	var b strings.Builder
	for i := range 300 {
		fmt.Fprintf(&b, "10:00:%02d INFO heartbeat ok seq=%d\n", i%60, i)
		if i >= 100 && i < 140 { // retries sit on lines 102..180, between heartbeats
			fmt.Fprintf(&b, "10:00:%02d INFO retry connecting to db attempt=%d\n", i%60, i)
		}
	}
	in := b.String()
	tests := []struct {
		name   string
		bursts bool
		want   string
		not    string
	}{
		{"off", false, "retry connecting to db attempt=100 (×40)", "mostly"},
		{"on names the stretch", true, "(×40; mostly lines 102–164)", "heartbeat ok seq=0 (×300; mostly"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.Log}}}
			(&Optimizer{cfg: &Config{Context: 0, DropDebug: true, MaxLines: 300, Bursts: tt.bursts}}).Optimize(nil, p)
			got := p.Blocks[0].Text
			if !strings.Contains(got, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, got)
			}
			if strings.Contains(got, tt.not) {
				t.Errorf("unexpected %q in:\n%s", tt.not, got)
			}
		})
	}
}

func TestBurst(t *testing.T) {
	seq := func(from, to, step int) []int {
		var s []int
		for i := from; i < to; i += step {
			s = append(s, i)
		}
		return s
	}
	tests := []struct {
		name        string
		at          []int
		total       int
		first, last int
		ok          bool
	}{
		{"packed", seq(100, 140, 1), 400, 100, 131, true},
		{"steady", seq(0, 400, 4), 400, 0, 0, false},
		{"too few", seq(100, 110, 1), 400, 0, 0, false},
		{"most packed, few strays", append(seq(100, 140, 1), 5, 390), 400, 100, 133, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := slices.Clone(tt.at)
			slices.Sort(at)
			first, last, ok := burst(at, len(at), tt.total)
			if ok != tt.ok || ok && (first != tt.first || last != tt.last) {
				t.Errorf("burst = %d %d %v, want %d %d %v", first, last, ok, tt.first, tt.last, tt.ok)
			}
		})
	}
}

func TestPassesSelectStages(t *testing.T) {
	in := "Oct 10 00:05:45 host systemd[1]: Starting WPA supplicant...\n" +
		"Oct 10 00:05:45 host systemd[1]: Started WPA supplicant.\n"
	tests := []struct {
		name   string
		passes []string
		want   bool // the Starting line survives
	}{
		{"default pairs units", nil, false},
		{"all passes", []string{"entries", "units", "prefixes"}, false},
		{"without units", []string{"entries", "prefixes"}, true},
		{"none", []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{Context: 2, DropDebug: true, MaxLines: 300, Passes: tt.passes}
			if tt.passes == nil {
				cfg.Passes = defaultPasses
			}
			p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.Journal}}}
			(&Optimizer{cfg: cfg}).Optimize(nil, p)
			if got := strings.Contains(p.Blocks[0].Text, "Starting WPA"); got != tt.want {
				t.Errorf("Starting line kept = %v, want %v:\n%s", got, tt.want, p.Blocks[0].Text)
			}
		})
	}
}

func TestPassesValidate(t *testing.T) {
	tests := []struct {
		name   string
		passes []string
		bad    bool
	}{
		{"default", defaultPasses, false},
		{"subset in order", []string{"entries", "prefixes"}, false},
		{"empty", []string{}, false},
		{"unknown", []string{"entries", "magic"}, true},
		{"out of order", []string{"prefixes", "entries"}, true},
		{"duplicate", []string{"units", "units"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Config{Context: 2, MaxLines: 300, Passes: tt.passes}
			if err := c.Validate(); (err != nil) != tt.bad {
				t.Fatalf("Validate() = %v, want error: %v", err, tt.bad)
			}
		})
	}
}
