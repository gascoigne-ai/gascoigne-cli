package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// seedGainLedger points the ledger at a throwaway file with one optimized run.
func seedGainLedger(t *testing.T) {
	t.Helper()
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	path := filepath.Join(t.TempDir(), "l.db")
	t.Setenv(ledger.EnvPath, path)
	store := ledger.New(&ledger.Config{Path: path})
	if err := store.Start(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	event := component.Event{Pipeline: "exec", Command: "go test", Type: "test/go", RawTokens: 1000, OutTokens: 100,
		TS: time.Now(), Key: "kexec", Outcome: component.Unchanged}
	if err := store.Record(t.Context(), event); err != nil {
		t.Fatal(err)
	}
	if err := store.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestGainModes(t *testing.T) {
	seedGainLedger(t)
	tests := []struct {
		name    string
		args    []string
		want    []string
		wantErr string
	}{
		{"overview", []string{"-all", "-color", "never"}, []string{"saved 900 tokens"}, ""},
		{"overview json", []string{"-all", "-json"}, []string{"{"}, ""},
		{"oneline", []string{"-all", "-oneline"}, []string{"gascoigne"}, ""},
		{"by json", []string{"-all", "-by", "command", "-json"}, []string{"go test"}, ""},
		{"by table", []string{"-all", "-by", "command", "-color", "never"}, []string{"go test"}, ""},
		{"refetch", []string{"-all", "-refetch"}, []string{"no optimized runs"}, ""},
		{"failures", []string{"-failures"}, nil, ""},
		{"failures json", []string{"-failures", "-json"}, nil, ""},
		{"failures csv", []string{"-failures", "-csv"}, nil, ""},
		{"csv alone", []string{"-csv"}, nil, "-csv needs -by or -failures"},
		{"bad since", []string{"-since", "soon"}, nil, "soon"},
		{"bad color", []string{"-color", "purple"}, nil, "purple"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			out := capture(t, func() { err = cmdGain(t.Context(), tt.args) })
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !strings.Contains(out, want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}
