package externaloptimizer

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// The shipped examples must keep working against real tool output.
func TestExamples(t *testing.T) {
	tests := []struct {
		name, protocol, fixture, typ string
		command                      []string
		need                         string // interpreter the example needs
		keep, gone                   string
	}{
		{"strip-ansi", Text, "test/vitest-fail.txt", "test/vitest",
			[]string{"sh", "../../examples/external-optimizers/strip-ansi.sh"}, "sh", "RUN  v3", "\x1b["},
		{"log-levels", JSON, "log/app.log", "log/app",
			[]string{"python3", "../../examples/external-optimizers/log-levels.py"}, "python3", "[log-levels] kept 66 of 1406", " INFO "},
		{"rpc-squash", RPC, "log/app.log", "log/app",
			[]string{"python3", "../../examples/external-optimizers/rpc-squash.py"}, "python3", "OrderService.place", ""},
		{"jq", Text, "log/structured.ndjson", "json",
			[]string{"jq", "-c", "del(..|nulls)"}, "jq", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := exec.LookPath(tt.need); err != nil {
				t.Skipf("%s not installed", tt.need)
			}
			data, err := os.ReadFile("../../testdata/eval/" + tt.fixture)
			if err != nil {
				t.Fatal(err)
			}
			cfg := &Config{Command: tt.command, Protocol: tt.protocol, Timeout: 20 * time.Second, MaxOutputBytes: 8 << 20}
			p := payload.New(payload.CmdOutput, string(data))
			p.Blocks[0].Type = tt.typ
			if err := newOpt(t, cfg).Optimize(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			got := p.Text()
			if tt.gone != "" && strings.Contains(got, tt.gone) {
				t.Errorf("output still has %q", tt.gone)
			}
			if !strings.Contains(got, tt.keep) {
				t.Errorf("output lost %q:\n%.300s", tt.keep, got)
			}
		})
	}
}
