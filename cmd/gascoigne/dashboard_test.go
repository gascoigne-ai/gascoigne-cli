package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

func testDashboard(t *testing.T) (*ledger.Ledger, http.Handler) {
	t.Helper()
	l := ledger.New(&ledger.Config{Path: filepath.Join(t.TempDir(), "l.db"), RawTTL: time.Hour})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Shutdown(context.Background()) })
	return l, dashboard(l, 10*time.Millisecond, serverInfo{PID: 42, Build: "b1", Ledger: l.Path()}, func() {})
}

func record(t *testing.T, l *ledger.Ledger, session, cmd string) {
	t.Helper()
	ev := component.Event{TS: time.Now(), Session: session, Agent: "claude", Command: cmd, Key: "cmd:" + cmd,
		Outcome: component.Optimized, RawTokens: 1000, OutTokens: 100, RawID: "r1",
		Steps: []component.Step{{Optimizer: "git", Family: "vcs", Dialect: "vcs/git-log", RawTokens: 1000, OutTokens: 100}}}
	if err := l.Record(t.Context(), ev); err != nil {
		t.Fatal(err)
	}
}

func get(h http.Handler, method, url string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, url, nil)
	r.Host = "127.0.0.1:7878"
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestDashboardPages(t *testing.T) {
	l, h := testDashboard(t)
	record(t, l, "s1", "git log")
	record(t, l, "s2", "go test")
	tests := []struct {
		name, url, want string
	}{
		{"page", "/", "<title>gascoigne"},
		{"events", "/api/events", `"command":"go test"`},
		{"events filtered", "/api/events?session=s1", `"command":"git log"`},
		{"overview", "/api/overview?since=7d", `"saved_tokens":1800`},
		{"model families", "/api/overview", `"family":"claude"`},
		{"pricing basis", "/api/overview", `official list prices`},
		{"sessions", "/api/sessions", `"group":"s1"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := get(h, "GET", tt.url)
			if w.Code != 200 || !strings.Contains(w.Body.String(), tt.want) {
				t.Fatalf("%d, want %q in:\n%s", w.Code, tt.want, w.Body.String())
			}
		})
	}
	if w := get(h, "GET", "/api/events?session=s1"); strings.Contains(w.Body.String(), "go test") {
		t.Fatalf("session filter ignored: %s", w.Body.String())
	}
	if w := get(h, "GET", "/api/overview?since=yesterday"); w.Code != 400 {
		t.Fatalf("bad since: %d", w.Code)
	}
}

func TestDashboardRawDoesNotMarkFailure(t *testing.T) {
	l, h := testDashboard(t)
	p := payload.New(payload.CmdOutput, "original text")
	id, err := l.SaveRaw(t.Context(), "cmd:git log", p)
	if err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/raw/"+id); w.Code != 200 || w.Body.String() != "original text" {
		t.Fatalf("%d %q", w.Code, w.Body.String())
	}
	if f, _ := l.Failures(t.Context(), "cmd:git log"); f.Count != 0 {
		t.Fatal("viewing an original in the dashboard must not mark its source as failing")
	}
	if w := get(h, "GET", "/api/raw/nope"); w.Code != 404 {
		t.Fatalf("missing raw: %d", w.Code)
	}
}

func TestDashboardForget(t *testing.T) {
	l, h := testDashboard(t)
	if err := l.MarkFailure(t.Context(), "cmd:git log", "manual"); err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/failures"); !strings.Contains(w.Body.String(), `"key":"cmd:git log"`) {
		t.Fatalf("failures: %s", w.Body.String())
	}
	if w := get(h, "POST", "/api/forget?key=cmd:git+log", "Origin", "http://evil.example"); w.Code != 403 {
		t.Fatalf("cross-origin forget: %d", w.Code)
	}
	if w := get(h, "POST", "/api/forget?key=cmd:git+log", "Origin", "http://127.0.0.1:7878"); w.Code != 204 {
		t.Fatalf("forget: %d %s", w.Code, w.Body.String())
	}
	if f, _ := l.Failures(t.Context(), "cmd:git log"); f.Count != 0 {
		t.Fatal("not forgotten")
	}
}

func TestDashboardOnlyLocalHosts(t *testing.T) {
	_, h := testDashboard(t)
	r := httptest.NewRequest("GET", "/api/events", nil)
	r.Host = "attacker.example:7878" // DNS rebinding
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("foreign Host: %d", w.Code)
	}
}

func TestDashboardStream(t *testing.T) {
	l, h := testDashboard(t)
	record(t, l, "s1", "git log")
	srv := httptest.NewServer(h)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	sc := bufio.NewScanner(resp.Body)
	var batches [][]eventJSON
	for sc.Scan() && len(batches) < 2 {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var evs []eventJSON
		if err := json.Unmarshal([]byte(data), &evs); err != nil {
			t.Fatal(err)
		}
		batches = append(batches, evs)
		if len(batches) == 1 {
			record(t, l, "s1", "go test") // arrives live
		}
	}
	if len(batches) != 2 || batches[0][0].Command != "git log" || batches[1][0].Command != "go test" {
		t.Fatalf("want the backlog, then the new event: %+v", batches)
	}
}

func TestServerControl(t *testing.T) {
	l, _ := testDashboard(t)
	stopped := make(chan struct{})
	h := dashboard(l, time.Second, serverInfo{PID: 42, Build: "b1", Ledger: l.Path()}, func() { close(stopped) })
	record(t, l, "s1", "git log")
	record(t, l, "s2", "go test")

	if w := get(h, "GET", "/api/health"); !strings.Contains(w.Body.String(), `"build":"b1"`) {
		t.Fatalf("health: %s", w.Body.String())
	}
	if w := get(h, "POST", "/api/reset?session=s1"); w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted_events":1`) {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
	if w := get(h, "GET", "/api/events"); strings.Contains(w.Body.String(), "git log") || !strings.Contains(w.Body.String(), "go test") {
		t.Fatalf("reset of s1 touched the wrong events: %s", w.Body.String())
	}
	if w := get(h, "POST", "/api/reset?session=s2&all=1"); w.Code != 400 {
		t.Fatalf("filtered full reset: %d", w.Code)
	}
	if w := get(h, "POST", "/api/shutdown", "Origin", "http://evil.example"); w.Code != 403 {
		t.Fatalf("cross-origin shutdown: %d", w.Code)
	}
	if w := get(h, "POST", "/api/shutdown"); w.Code != 202 {
		t.Fatalf("shutdown: %d", w.Code)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("shutdown did not stop the server")
	}
}

func TestFlushOriginals(t *testing.T) {
	l, _ := testDashboard(t)
	h := dashboard(l, time.Second, serverInfo{}, func() {})
	if _, err := l.SaveRaw(t.Context(), "cmd:ls", payload.New(payload.CmdOutput, "a\nb")); err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/originals"); !strings.Contains(w.Body.String(), `"count":1`) {
		t.Fatalf("originals: %s", w.Body.String())
	}
	if w := get(h, "POST", "/api/flush"); w.Code != 200 || !strings.Contains(w.Body.String(), `"deleted_originals":1`) {
		t.Fatalf("flush: %d %s", w.Code, w.Body.String())
	}
	if w := get(h, "GET", "/api/originals"); !strings.Contains(w.Body.String(), `"count":0`) {
		t.Fatalf("after flush: %s", w.Body.String())
	}
}

func TestServerAction(t *testing.T) {
	built := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	info := serverInfo{Build: "b1", Modified: built, Ledger: "/data/l.db"}
	newer, older := built.Add(time.Hour), built.Add(-time.Hour)
	tests := []struct {
		name     string
		info     serverInfo
		running  bool
		ledger   string
		build    string
		modified time.Time
		want     action
	}{
		{"none running", info, false, "/data/l.db", "b1", built, startServer},
		{"same build and ledger", info, true, "/data/l.db", "b1", built, keepServer},
		{"rebuilt binary", info, true, "/data/l.db", "b2", newer, replaceServer},
		{"older binary", info, true, "/data/l.db", "b0", older, keepServer},
		{"server without mtime", serverInfo{Build: "b1", Ledger: "/data/l.db"}, true, "/data/l.db", "b2", older, replaceServer},
		{"another ledger", info, true, "/tmp/eval.db", "b2", newer, keepServer},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serverAction(tt.info, tt.running, tt.ledger, tt.build, tt.modified); got != tt.want {
				t.Fatalf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWatchBinary(t *testing.T) {
	tests := []struct {
		name  string
		reads []string
		want  bool
	}{
		{"unchanged", []string{"b1", "b1", "b1", "b1"}, false},
		{"replaced", []string{"b1", "b2", "b2"}, true},
		{"still writing", []string{"b2", "b3", "unknown", "b4", "b1"}, false},
		{"missing for a moment", []string{"unknown", "unknown", "b1"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			reads := make(chan string, len(tt.reads))
			for _, r := range tt.reads {
				reads <- r
			}
			read := func() string {
				select {
				case r := <-reads:
					return r
				default:
					cancel() // every read was seen
					return "b1"
				}
			}
			changed := false
			watchBinary(ctx, "b1", time.Millisecond, read, func() { changed = true })
			if changed != tt.want {
				t.Fatalf("changed = %v, want %v", changed, tt.want)
			}
		})
	}
}

func TestOptimizedOutputEndpoint(t *testing.T) {
	l, h := testDashboard(t)
	id, err := l.SaveRaw(t.Context(), "cmd:x", payload.New(payload.CmdOutput, "long original\n"))
	if err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/out/"+id); w.Code != 404 {
		t.Fatalf("before a run: %d", w.Code)
	}
	if err := l.Record(t.Context(), component.Event{TS: time.Now(), RawID: id, Outcome: component.Optimized, Output: "short\n"}); err != nil {
		t.Fatal(err)
	}
	if w := get(h, "GET", "/api/out/"+id); w.Code != 200 || w.Body.String() != "short\n" {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestDashboardHidesInstructionText(t *testing.T) {
	l, h := testDashboard(t)
	id, err := l.SaveRaw(t.Context(), "prompt:0a1b2c3d", payload.New(payload.Message, "You are an agent."))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/raw/", "/api/out/"} {
		if w := get(h, "GET", path+id); w.Code != 403 || strings.Contains(w.Body.String(), "You are an agent") {
			t.Fatalf("%s: %d %q", path, w.Code, w.Body.String())
		}
	}
}

func TestRunServerRefusesNonLoopback(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:0", ":0", "192.0.2.1:0", "example.com:0"} {
		t.Run(addr, func(t *testing.T) {
			err := runServer(t.Context(), nil, addr, 0)
			if err == nil || !strings.Contains(err.Error(), "loopback") {
				t.Fatalf("addr %q: want loopback error, got %v", addr, err)
			}
		})
	}
}

func TestLocalOnlySecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	localOnly(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "http://localhost/", nil))
	for h, want := range map[string]string{"X-Content-Type-Options": "nosniff", "X-Frame-Options": "DENY"} {
		if got := rec.Header().Get(h); got != want {
			t.Errorf("%s = %q, want %q", h, got, want)
		}
	}
}

func TestDashboardMCPReview(t *testing.T) {
	l, h := testDashboard(t)
	t.Run("empty review has lists, not null", func(t *testing.T) {
		w := get(h, "GET", "/api/mcp")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"servers":[]`) || strings.Contains(w.Body.String(), "null") {
			t.Fatalf("%d: %s", w.Code, w.Body.String())
		}
	})
	if err := l.RecordToolList(t.Context(), "claude", "github", map[string]int{"search": 300, "create": 500}); err != nil {
		t.Fatal(err)
	}
	if err := l.RecordToolCall(t.Context(), "claude", "github", "search"); err != nil {
		t.Fatal(err)
	}
	t.Run("review", func(t *testing.T) {
		w := get(h, "GET", "/api/mcp")
		var r mcpReview
		if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil || w.Code != 200 {
			t.Fatalf("%d %v: %s", w.Code, err, w.Body.String())
		}
		if len(r.Servers) != 1 || r.Servers[0].Name != "github" || r.Servers[0].Tokens != 800 || r.Servers[0].Calls7 != 1 {
			t.Fatalf("got %+v", r.Servers)
		}
	})
	t.Run("page", func(t *testing.T) {
		w := get(h, "GET", "/mcp")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "<title>gascoigne · MCP servers") {
			t.Fatalf("%d: %.200s", w.Code, w.Body.String())
		}
	})
	t.Run("page is local only", func(t *testing.T) {
		for _, url := range []string{"/mcp", "/api/mcp"} {
			r := httptest.NewRequest("GET", url, nil)
			r.Host = "attacker.example:7878" // DNS rebinding
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 403 {
				t.Fatalf("%s with a foreign Host: %d", url, w.Code)
			}
		}
	})
}
