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
