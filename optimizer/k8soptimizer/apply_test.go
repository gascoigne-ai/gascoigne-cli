package k8soptimizer

import (
	"slices"
	"testing"
)

func TestApply(t *testing.T) {
	in := []string{
		"service/a unchanged",
		"deployment.apps/a configured",
		"service/b unchanged",
		"deployment.apps/b unchanged",
		`pod "old" deleted`,
		"Error from server (NotFound): namespaces \"x\" not found",
		"service/c unchanged",
	}
	want := []string{
		"unchanged ×4 (service ×3, deployment.apps ×1)",
		"deployment.apps/a configured",
		`pod "old" deleted`,
		"Error from server (NotFound): namespaces \"x\" not found",
	}
	if got := apply(in); !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRollout(t *testing.T) {
	wait := func(n string) string {
		return `Waiting for deployment "api" rollout to finish: ` + n + " old replicas are pending termination..."
	}
	tests := []struct {
		name     string
		in, want []string
	}{
		{"success", []string{wait("2"), wait("1"), `deployment "api" successfully rolled out`},
			[]string{`deployment "api" successfully rolled out (after 2 progress lines)`}},
		{"failure keeps last", []string{wait("2"), wait("1"), `error: deployment "api" exceeded its progress deadline`},
			[]string{wait("1") + " (×2)", `error: deployment "api" exceeded its progress deadline`}},
		{"single", []string{wait("1")}, []string{wait("1")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rollout(tt.in); !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
