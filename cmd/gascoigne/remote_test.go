package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/hookreceiver"
)

func TestProcessEnv(t *testing.T) {
	tests := []struct {
		env  []string
		want bool
	}{
		{[]string{"HOME=/home/user", "GASCOIGNE_AGENT=claude", "GASCOIGNE_SET=a=1"}, false},
		{[]string{"GASCOIGNE_RAW="}, false},
		{[]string{"GASCOIGNE_RAW=1"}, true},
		{[]string{"GASCOIGNE_CAT_MODE=full"}, true},
		{[]string{"GASCOIGNE_LEDGER=/tmp/l.db"}, true},
	}
	for _, tt := range tests {
		if got := processEnv(tt.env); got != tt.want {
			t.Errorf("processEnv(%q) = %v, want %v", tt.env, got, tt.want)
		}
	}
}

func TestServerEnv(t *testing.T) {
	got := serverEnv([]string{"HOME=/home/user", "GASCOIGNE_RAW=1", "GASCOIGNE_AGENT=claude",
		"GASCOIGNE_LEDGER=/tmp/l.db", "GASCOIGNE_SERVER_ADDR=127.0.0.1:1"})
	want := []string{"HOME=/home/user", "GASCOIGNE_LEDGER=/tmp/l.db", "GASCOIGNE_SERVER_ADDR=127.0.0.1:1"}
	if !slices.Equal(got, want) {
		t.Fatalf("serverEnv = %q, want %q", got, want)
	}
}

// remoteSetup gives a config with its own ledger and a server on it whose
// requests are counted; build is what the server says it runs.
func remoteSetup(t *testing.T, build string) (cfgFile, ledgerFile string, hits *atomic.Int32) {
	t.Helper()
	for _, env := range []string{"GASCOIGNE_SERVER", "GASCOIGNE_LEDGER", "GASCOIGNE_RAW", "GASCOIGNE_CAT_MODE"} {
		t.Setenv(env, "")
	}
	dir := t.TempDir()
	ledgerFile = filepath.Join(dir, "l.db")
	cfgFile = filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfgFile, []byte("extensions: { ledger: { path: "+ledgerFile+" } }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	hits = &atomic.Int32{}
	w := newWarm(context.Background(), build, ledgerFile)
	t.Cleanup(w.Close)
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		localOnly(w).ServeHTTP(rw, r)
	}))
	t.Cleanup(srv.Close)
	t.Setenv("GASCOIGNE_SERVER_ADDR", strings.TrimPrefix(srv.URL, "http://"))
	return cfgFile, ledgerFile, hits
}

// hookRun optimizes text through the hook pipeline as the CLI does.
func hookRun(t *testing.T, cfgFile string) *payload.Payload {
	t.Helper()
	text, err := os.ReadFile("../../testdata/eval/go/test-panic-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	c := common{config: cfgFile, attrs: attrFlag{}, session: "s1", useServer: true, pipeline: "hook"}
	svc, err := c.start(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Shutdown(context.Background())
	out := &hookreceiver.Output{Text: string(text), Cmd: "go test ./..."}
	if err := svc.Run(hookreceiver.With(t.Context(), out), "hook"); err != nil {
		t.Fatal(err)
	}
	if out.Result == nil || out.Result.Attrs[payload.AttrRawID] == "" {
		t.Fatalf("not optimized: %+v", out.Result)
	}
	return out.Result
}

func TestRemoteOptimizes(t *testing.T) {
	tests := []struct {
		name     string
		build    string
		env      string
		wantHits int32
	}{
		{"served", build(), "", 1},
		{"other build: optimized here", "old", "", 1},
		{"process env: never asks", build(), "GASCOIGNE_CAT_MODE=clean", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfgFile, ledgerFile, hits := remoteSetup(t, tt.build)
			if k, v, ok := strings.Cut(tt.env, "="); ok {
				t.Setenv(k, v)
			}
			hookRun(t, cfgFile)
			if hits.Load() != tt.wantHits {
				t.Fatalf("server got %d requests, want %d", hits.Load(), tt.wantHits)
			}
			// Either way the run is recorded once, in the config's ledger.
			if n := countEvents(t, ledgerFile); n != 1 {
				t.Fatalf("%d events recorded, want 1", n)
			}
		})
	}
}

func TestRemoteWithoutServer(t *testing.T) {
	cfgFile, ledgerFile, _ := remoteSetup(t, build())
	t.Setenv("GASCOIGNE_SERVER_ADDR", "127.0.0.1:1") // nothing listens
	hookRun(t, cfgFile)
	if n := countEvents(t, ledgerFile); n != 1 {
		t.Fatalf("%d events recorded, want 1", n)
	}
}

func countEvents(t *testing.T, ledgerFile string) int {
	t.Helper()
	l := ledger.New(&ledger.Config{Path: ledgerFile})
	if err := l.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	defer l.Shutdown(context.Background())
	entries, err := l.Recent(t.Context(), component.StatsQuery{}, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
