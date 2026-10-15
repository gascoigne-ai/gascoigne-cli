package detect

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestRows(t *testing.T) {
	for _, r := range rows {
		t.Run(r.Type+"/"+r.Cmd+r.Sniff, func(t *testing.T) {
			if len(r.Cmds) > 0 {
				if r.Cmd == "" {
					t.Fatal("no command example")
				}
				if got := FromCmd(r.Cmd); got != r.Type {
					t.Errorf("command %q detected as %q", r.Cmd, got)
				}
			}
			if r.Sniff != "" {
				if r.Sample == "" {
					t.Fatal("no content example")
				}
				if got := Sniff(r.Sample); got != r.Type {
					t.Errorf("content sniffed as %q", got)
				}
			}
		})
	}
}

// TestRowCoverage reports rows without an eval case (run with -v). It does
// not fail: fixtures need the tool's real output, and a row is useful for
// detection before one exists.
func TestRowCoverage(t *testing.T) {
	b, err := os.ReadFile("../../testdata/eval/cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, r := range rows {
		if !strings.Contains(string(b), "expect_type: "+r.Type+"\n") && !slices.Contains(missing, r.Type) {
			missing = append(missing, r.Type)
		}
	}
	t.Logf("%d of %d row types have no eval case: %s", len(missing), len(rows), strings.Join(missing, " "))
}
