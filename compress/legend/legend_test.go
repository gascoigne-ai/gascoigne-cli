package legend

import "testing"

func TestRenderExpand(t *testing.T) {
	tests := []struct {
		name string
		kind string
		defs []Def
		body string
		want string
	}{
		{"paths", Paths, []Def{{"§1", "/src/app/internal"}}, "§1/a.go:3 §1/b.go:4", "/src/app/internal/a.go:3 /src/app/internal/b.go:4"},
		{"rule with spaces", Rules, []Def{{"¶1", "unused variable (ineffassign)"}, {"¶2", "x is not checked"}}, "a.go:1 ¶1\nb.go:2 ¶2", "a.go:1 unused variable (ineffassign)\nb.go:2 x is not checked"},
		{"ten ids", Names, []Def{{"†1", "one"}, {"†10", "ten"}}, "†1 †10", "one ten"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := Render(tt.kind, tt.defs)
			if !Is(line) || !Has(line) {
				t.Fatalf("Is(%q) = false", line)
			}
			got := Expand(line + "\n" + tt.body)
			if want := Expand(line) + "\n" + tt.want; got != want {
				t.Errorf("got %q, want %q", got, want)
			}
		})
	}
}

func TestIs(t *testing.T) {
	for line, want := range map[string]bool{
		"[paths: §1=/a/b/c]":      true,
		"[rules: ¶1=msg]":         true,
		"[names: †1=api-1]":       true,
		"x [paths: §1=/a/b/c]":    false,
		"[sources: a=pod/x]":      false,
		"[↑ 3 lines as in a run]": false,
	} {
		if got := Is(line); got != want {
			t.Errorf("Is(%q) = %v, want %v", line, got, want)
		}
	}
}

func TestWorth(t *testing.T) {
	if got := Worth(3, 30, 2); got != 3*28-2-30-2 {
		t.Errorf("Worth = %d", got)
	}
	if Worth(1, 10, 2) >= 0 {
		t.Error("one use of a short value should be a loss")
	}
}
