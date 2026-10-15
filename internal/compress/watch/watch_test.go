package watch

import (
	"slices"
	"strings"
	"testing"
)

func TestEnd(t *testing.T) {
	for _, l := range []string{
		"[7:00:03 AM] Found 1 error. Watching for file changes.",
		"\x1b[90m[7:00:03 AM]\x1b[0m Found 0 errors. Watching for file changes.",
		"webpack 5.94.0 compiled with 2 errors in 1203 ms",
		"webpack 5.94.0 compiled successfully in 88 ms",
		"<i> [webpack-dev-middleware] wait until bundle finished",
		"compiled successfully in 412 ms",
		"built in 812ms.",
		"[2026-10-10 07:00:00] waiting for changes...",
		"[watch] build finished, watching for changes...",
		"[Finished running. Exit status: 101]",
	} {
		want := !strings.Contains(l, "wait until")
		if End(l) != want {
			t.Errorf("End(%q) = %v", l, !want)
		}
	}
}

func TestLast(t *testing.T) {
	in := []string{
		"[7:00:01 AM] Starting compilation in watch mode...",
		"src/a.ts(3,5): error TS2322: Type 'string' is not assignable to type 'number'.",
		"[7:00:03 AM] Found 1 error. Watching for file changes.",
		"[7:01:10 AM] File change detected. Starting incremental compilation...",
		"src/b.ts(9,1): error TS2304: Cannot find name 'x'.",
		"[7:01:11 AM] Found 1 error. Watching for file changes.",
		"[7:02:00 AM] File change detected. Starting incremental compilation...",
		"[7:02:01 AM] Found 0 errors. Watching for file changes.",
	}
	got := Last(in)
	want := []string{
		"… 2 earlier watch cycles omitted (6 lines), results:",
		"  [7:00:03 AM] Found 1 error. Watching for file changes.",
		"  [7:01:11 AM] Found 1 error. Watching for file changes.",
		"[7:02:00 AM] File change detected. Starting incremental compilation...",
		"[7:02:01 AM] Found 0 errors. Watching for file changes.",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got\n%s", strings.Join(got, "\n"))
	}
	if again := Last(got); !slices.Equal(again, got) {
		t.Fatalf("not idempotent:\n%s", strings.Join(again, "\n"))
	}
	if one := in[:3]; !slices.Equal(Last(one), one) {
		t.Fatal("single cycle changed")
	}
}
