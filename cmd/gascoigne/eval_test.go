package main

import (
	"errors"
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

func TestLiveCase(t *testing.T) {
	tests := []struct {
		name, run, want string
		err             error
	}{
		{"output", "echo hello", "hello\n", nil},
		{"failing command is still a case", "echo boom; exit 3", "boom\n", nil},
		{"missing program skips", "gascoigne-no-such-program-xyz", "", errNotInstalled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := (&evalCase{Run: tt.run}).live(t.Context())
			if !errors.Is(err, tt.err) {
				t.Fatalf("err %v, want %v", err, tt.err)
			}
			if err == nil && (p.Text() != tt.want || p.Attrs["cmd"] != tt.run) {
				t.Errorf("text %q cmd %q", p.Text(), p.Attrs["cmd"])
			}
		})
	}
}

func TestWordOverlap(t *testing.T) {
	o := wordOverlap("The cat sat. The end", "the CAT, the mat; menu")
	// ref: the×2 cat sat end (5); out: the×2 cat mat menu (5); shared: the×2 cat.
	if o != (overlap{ref: 5, out: 5, both: 3}) || o.recall() != 0.6 || o.precision() != 0.6 {
		t.Errorf("overlap %+v", o)
	}
}
