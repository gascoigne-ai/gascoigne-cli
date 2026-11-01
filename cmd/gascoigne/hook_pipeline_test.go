//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A wrapped pipeline must keep the exit status and (through a run that passes
// text on) the output of the original, also when a middle stage fails or the
// last one finds nothing. Runs under bash: wrapPipeline reads PIPESTATUS.
func TestWrapPipelineBashParity(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gascoigne"), []byte("#!/bin/sh\n[ \"$1\" = run ] && exec cat\n"+strings.TrimPrefix(execStub, "#!/bin/sh\n")), 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(line string) (string, int) {
		cmd := exec.Command("bash", "-c", line)
		cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "LC_ALL=C")
		out, err := cmd.Output()
		var ee *exec.ExitError
		switch {
		case errors.As(err, &ee):
			return string(out), ee.ExitCode()
		case err != nil:
			t.Fatal(err)
		}
		return string(out), 0
	}
	lines := []string{
		`seq 1 5 | grep 3`,
		`seq 1 5 | grep zzz`,
		`seq 1 20 | grep -c 1 | sort -u`,
		`sh -c 'echo x; exit 4' | cat`,
		`seq 1 9 | sort -r | uniq -c | sort -rn`,
		`seq 1 3 | sh -c 'cat; exit 3'`,
		`sh -c 'exit 2' && seq 1 3 | grep 1`,
		`sh -c 'exit 0' && seq 1 3 | grep 9`,
		`sh -c 'exit 2'; seq 1 3 | grep '1|2'`,
		`seq 1 30 | grep -F "1" | wc -l`,
		`seq 1 30 | head -5 | grep 3`,
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			got, ok := wrapCommand(line, "gascoigne exec")
			if !ok {
				t.Fatalf("not wrapped")
			}
			wantOut, wantCode := run(line)
			gotOut, gotCode := run(got)
			if gotOut != wantOut || gotCode != wantCode {
				t.Fatalf("rewrite changed behavior\nrewrite: %s\nwant: %q exit %d\ngot:  %q exit %d", got, wantOut, wantCode, gotOut, gotCode)
			}
		})
	}
}
