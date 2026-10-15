package taskoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func check(t *testing.T, got []string, keep, drop []string) {
	t.Helper()
	out := compress.Join(got)
	for _, k := range keep {
		if !strings.Contains(out, k) {
			t.Errorf("lost %q in:\n%s", k, out)
		}
	}
	for _, d := range drop {
		if strings.Contains(out, d) {
			t.Errorf("kept %q in:\n%s", d, out)
		}
	}
}

func TestTurbo(t *testing.T) {
	in := compress.Lines("• Running build in 2 packages\na:build: cache hit, replaying logs 1\na:build: > tsc\nb:build: cache miss, executing 2\nb:build: > tsc\nb:build: src/x.ts(1,1): error TS2304: Cannot find name 'y'.\nb#build: command (/r/b) npm run build exited (1)\n Tasks:    1 successful, 2 total\nFailed:    b#build\n")
	check(t, turbo(in), []string{"cached: a:build", "failed: b:build", "1:1 error TS2304 Cannot find name 'y'.", "Failed:    b#build"}, []string{"b#build: command"})
}

func TestMakeKeepsOwnLinesLast(t *testing.T) {
	in := compress.Lines("make[1]: Entering directory '/x'\ncc -c a.c\na.c:1:1: error: boom\nmake[1]: Leaving directory '/x'\nmake: *** [all] Error 2\n")
	got := makeLike(in)
	check(t, got, []string{"1:1 error boom", "make: *** [all] Error 2"}, []string{"Entering directory"})
	if got[len(got)-1] != "make: *** [all] Error 2" {
		t.Errorf("make status not last: %q", got)
	}
}

func TestCILog(t *testing.T) {
	in := compress.Lines("build\tSet up job\t2026-10-10T08:00:00.1Z Runner\nbuild\tRun go test ./...\t2026-10-10T08:00:01.1Z --- FAIL: TestX (0.00s)\nbuild\tRun go test ./...\t2026-10-10T08:00:01.2Z FAIL\tex.com/x\t0.1s\nbuild\tRun go test ./...\t2026-10-10T08:00:01.3Z ##[error]Process completed with exit code 1.\n")
	check(t, ciLog(in), []string{"succeeded: build / Set up job (1 lines)", "failed: build / Run go test ./...", "--- FAIL: TestX", "exit code 1"}, []string{"2026-10-10T08"})
}

func TestComposeUp(t *testing.T) {
	in := compress.Lines(" ✔ Container a-1  Started\n ✔ Container b-1  Started\nAttaching to a-1, b-1\na-1  | ok 1\na-1  | ERROR boom\nb-1  | ready\n")
	check(t, composeUp(in), []string{"container started ×2", "a-1:", "ERROR boom", "b-1:"}, []string{"✔ Container"})
}
