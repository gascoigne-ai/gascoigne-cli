package main

import (
	"maps"
	"path/filepath"
	"testing"
)

func TestBaselineRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.tsv")
	if m, err := readBaseline(path); err != nil || m != nil {
		t.Fatalf("missing file: %v, %v", m, err)
	}
	want := map[string]baseline{"go/test-fail": {185, 1, 2}, "log/app": {90, 0, 7}}
	if err := writeBaseline(path, []string{"go/test-fail", "log/app"}, want); err != nil {
		t.Fatal(err)
	}
	got, err := readBaseline(path)
	if err != nil || !maps.Equal(got, want) {
		t.Errorf("got %v, %v; want %v", got, err, want)
	}
}

func TestBaselineDiff(t *testing.T) {
	tests := []struct {
		name     string
		cur, old baseline
		want     string
	}{
		{"same", baseline{10, 1, 2}, baseline{10, 1, 2}, ""},
		{"more tokens", baseline{12, 1, 2}, baseline{10, 1, 2}, "out tokens 10→12"},
		{"retention", baseline{10, 3, 0}, baseline{10, 1, 2}, "lost errors 1→3, lost artifacts 2→0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cur.diff(tt.old); got != tt.want {
				t.Errorf("diff = %q, want %q", got, tt.want)
			}
		})
	}
}
