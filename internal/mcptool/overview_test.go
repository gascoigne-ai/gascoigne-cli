package mcptool

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

func TestEstimates(t *testing.T) {
	plain := estimates(Overview{})
	if len(plain) != 1 || !strings.HasPrefix(plain[0], "tokens: estimated") {
		t.Fatalf("plain: %q", plain)
	}
	o := Overview{Cost: Costs{Rereads: 110, Savings: []cost.Saving{{}}}, Optimizers: []StatsJSON{{Group: "image"}, {Group: "tools"}, {Group: "grep"}}}
	got := strings.Join(estimates(o), "\n")
	for _, want := range []string{"reread 110 times", "pixels/750", "once per distinct set"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
}
