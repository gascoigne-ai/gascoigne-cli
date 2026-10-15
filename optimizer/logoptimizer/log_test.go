package logoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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
