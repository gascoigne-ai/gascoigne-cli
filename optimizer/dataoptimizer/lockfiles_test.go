package dataoptimizer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTextLockfilesListNameVersion(t *testing.T) {
	tests := []struct {
		file   string
		header string
		want   []string
		never  string // dropped content
	}{
		{"pnpm-lock.yaml", "pnpm-lock.yaml: ", []string{"\n@pnpm/exe.android-arm64 12.11.2\n"}, "integrity"},
		{"pubspec.lock", "pubspec.lock: ", []string{"\nadaptive_breakpoints 0.1.7\n"}, "sha256"},
		{"mix.lock", "mix.lock: ", []string{"\nbandit 1.12.4\n", "\ncastore 1.0.20\n"}, "hexpm"},
		{"Podfile.lock", "Podfile.lock: ", []string{"\nboost 1.84.0\n", "\nhermes-engine/Pre-built 260318099.0.1\n"}, "CHECKSUMS"},
		{"Package.resolved", "Package.resolved: ", []string{"\naexml 4.7.0\n"}, "revision"},
		{"bun.lock", "bun.lock: ", []string{"\n@ark/schema 0.55.0\n"}, "sha512"},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			in, err := os.ReadFile(filepath.Join("..", "..", "testdata", "eval", "lock", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			got := lock(string(in))
			if !strings.HasPrefix(got, tc.header) {
				t.Fatalf("header: got %.80q, want prefix %q", got, tc.header)
			}
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q", w)
				}
			}
			if strings.Contains(got, tc.never) {
				t.Errorf("%q not dropped", tc.never)
			}
			if lock(got) != got {
				t.Error("not idempotent")
			}
		})
	}
}
