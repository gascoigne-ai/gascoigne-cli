package probe

import (
	"slices"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		orig, out     string
		lostErrors    []string
		lostArtifacts []string
	}{
		{
			name: "verbatim",
			orig: "ok\nmain.go:3: undefined: x\n",
			out:  "main.go:3: undefined: x\n",
		},
		{
			name:       "error line dropped",
			orig:       "a.go:3: undefined: x\nb.go:9: cannot use y\n",
			out:        "a.go:3: undefined: x\n",
			lostErrors: []string{"b.go:9: cannot use y"},
		},
		{
			name: "diagnostics grouped by file",
			orig: "pkg/a.go:3:1: undefined: x\npkg/a.go:7:2: undefined: y\n",
			out:  "pkg/a.go:\n  3:1: undefined: x\n  7:2: undefined: y\n",
		},
		{
			name: "path legend expanded",
			orig: "/home/u/src/shop/orders.go:14 panic: boom\n",
			out:  "[paths: §1=/home/u/src/shop]\n§1/orders.go:14 panic: boom\n",
		},
		{
			name: "legend ids longer first",
			orig: "/x/y/z/a.go\n",
			out:  "[paths: §1=/q §12=/x/y/z]\n§12/a.go\n",
		},
		{
			name:          "listing grouped by directory",
			orig:          "src/app/main.go\nsrc/app/util.go\nsrc/lib/gone.go\n",
			out:           "src/app/: main.go util.go\n",
			lostArtifacts: []string{"src/lib/gone.go"},
		},
		{
			name: "hash by short form",
			orig: "commit 3f9a2c41d0e8b7a6c5d4e3f2a1b0c9d8e7f6a5b4\n",
			out:  "3f9a2c4 fix things\n",
		},
		{
			name: "words are not hashes",
			orig: "the defaced decade\n",
			out:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Run(tt.orig, tt.out)
			if !slices.Equal(r.Errors.Lost, tt.lostErrors) {
				t.Errorf("lost errors %q, want %q", r.Errors.Lost, tt.lostErrors)
			}
			if !slices.Equal(r.Artifacts.Lost, tt.lostArtifacts) {
				t.Errorf("lost artifacts %q, want %q", r.Artifacts.Lost, tt.lostArtifacts)
			}
		})
	}
}

func TestNumerics(t *testing.T) {
	r := Run(`{"retries": 3, "port": 8787}`, "_cols: retries port\n_rows: 3 8080\n")
	if r.Numerics.Total != 2 || !slices.Equal(r.Numerics.Lost, []string{`port": 8787`}) {
		t.Errorf("numerics %+v", r.Numerics)
	}
}
