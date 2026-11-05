package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/inproc"
)

// sdkServer serves the /sdk/v1 routes on a config with its own ledger.
func sdkServer(t *testing.T) *httptest.Server {
	t.Helper()
	sandbox(t) // changes directory: fixtures are read by absolute path
	dir := t.TempDir()
	ledgerFile := filepath.Join(dir, "l.db")
	cfgFile := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("extensions: { ledger: { path: "+ledgerFile+" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GASCOIGNE_CONFIG", cfgFile)
	l := ledger.New(&ledger.Config{Path: ledgerFile})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Shutdown(context.Background()) })
	w := newWarm(context.Background(), "b", ledgerFile)
	t.Cleanup(w.Close)
	mux := http.NewServeMux()
	mux.Handle("POST /sdk/v1/optimize", sdkOptimize(w))
	mux.Handle("GET /sdk/v1/raw/{id}", sdkRaw(l))
	mux.Handle("GET /sdk/v1/stats", sdkStats(l))
	mux.Handle("GET /sdk/v1/recent", sdkRecent(l))
	mux.Handle("GET /sdk/v1/health", sdkHealthHandler("test", true))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestSDKHealth(t *testing.T) {
	srv := sdkServer(t)
	resp, err := http.Get(srv.URL + "/sdk/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var h sdkHealth
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		t.Fatal(err)
	}
	if h.Proto != sdkProto || h.Version != "test" || !h.Proxy {
		t.Fatalf("health = %+v", h)
	}
}

func TestSDKOptimizeAndRaw(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	srv := sdkServer(t)
	text, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(sdkRequest{Text: string(text), Cmd: "go test ./...", Agent: "langgraph"})
	resp, err := http.Post(srv.URL+"/sdk/v1/optimize", "application/json", strings.NewReader(string(body)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %s", resp.Status)
	}
	var out sdkResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.RawID == "" || len(out.Text) >= len(text) || out.OutTokens >= out.RawTokens {
		t.Fatalf("not optimized: %+v", out)
	}
	raw, err := http.Get(srv.URL + "/sdk/v1/raw/" + out.RawID)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Body.Close()
	got, err := io.ReadAll(raw.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(text) {
		t.Fatalf("raw is not the original (%d vs %d bytes)", len(got), len(text))
	}
}

func TestSDKOptimizeBadRequest(t *testing.T) {
	srv := sdkServer(t)
	resp, err := http.Post(srv.URL+"/sdk/v1/optimize", "application/json", strings.NewReader("{"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status %s, want 400", resp.Status)
	}
}

func TestSDKStatsAndRecent(t *testing.T) {
	fixture, err := filepath.Abs("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	srv := sdkServer(t)
	text, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"s1", "s2"} {
		body, _ := json.Marshal(sdkRequest{Text: string(text), Cmd: "go test ./...", Agent: "langgraph", Session: session})
		resp, err := http.Post(srv.URL+"/sdk/v1/optimize", "application/json", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}

	var stats []inproc.Stat
	getJSON(t, srv.URL+"/sdk/v1/stats?by=agent", &stats)
	if len(stats) != 1 || stats[0].Group != "langgraph" || stats[0].Runs != 2 || stats[0].Saved <= 0 {
		t.Fatalf("stats = %+v", stats)
	}
	var runs []inproc.Run
	getJSON(t, srv.URL+"/sdk/v1/recent?session=s1&limit=5", &runs)
	if len(runs) != 1 || runs[0].Session != "s1" || runs[0].RawID == "" {
		t.Fatalf("recent = %+v", runs)
	}
	for _, bad := range []string{"/sdk/v1/stats?by=nonsense", "/sdk/v1/stats?since=yesterday", "/sdk/v1/recent?limit=x", "/sdk/v1/recent?before=-1"} {
		resp, err := http.Get(srv.URL + bad)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status %s, want 400", bad, resp.Status)
		}
	}
}

func getJSON(t *testing.T, url string, into any) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: status %s", url, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatal(err)
	}
}

// A limit beyond what fits 16 bits is refused and one beyond what the ledger pages by is capped, so
// neither can wrap into a negative int on a 32-bit build (CodeQL go/incorrect-integer-conversion).
func TestSDKRecentBoundsItsNumbers(t *testing.T) {
	srv := sdkServer(t)
	for _, query := range []string{"limit=99999999999", "limit=4294967295", "limit=2147483648", "before=4294967295", "before=99999999999"} {
		resp, err := http.Get(srv.URL + "/sdk/v1/recent?" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: status %s", query, resp.Status)
		}
	}
	var runs []inproc.Run
	getJSON(t, srv.URL+"/sdk/v1/recent?limit=65535", &runs) // capped by the engine, not refused
	resp, err := http.Get(srv.URL + "/sdk/v1/recent?limit=65536")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("limit=65536: status %s, want 400", resp.Status)
	}
}
