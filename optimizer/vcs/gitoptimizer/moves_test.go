package gitoptimizer

import (
	"slices"
	"strings"
	"testing"
)

func TestMoves(t *testing.T) {
	block := []string{"func total(items []Item) int {", "\tsum := 0", "\tfor _, it := range items {", "\t\tsum += it.Price * it.Qty", "\t}", "\treturn sum", "}"}
	with := func(sign, indent string) []string {
		var out []string
		for _, l := range block {
			out = append(out, sign+indent+l)
		}
		return out
	}
	diff := func(parts ...[]string) []string { return slices.Concat(parts...) }
	header := func(file, hunk string) []string {
		return []string{"diff --git a/" + file + " b/" + file, "--- a/" + file, "+++ b/" + file, hunk}
	}
	tests := []struct {
		name  string
		lines []string
		want  string // "" = unchanged
	}{
		{"across files", diff(header("cart.go", "@@ -10,8 +10,1 @@"), []string{" package cart"}, with("-", ""),
			header("price/total.go", "@@ -0,0 +1,9 @@"), []string{"+package price", "+"}, with("+", "")),
			"-… 7 lines moved to price/total.go:3"},
		{"within a file, reindented", diff(header("cart.go", "@@ -10,8 +10,8 @@"), with("-", ""), []string{" var x = 1"}, with("+", "\t")),
			"-… 7 lines moved to line 11, reindented"},
		{"only added", diff(header("cart.go", "@@ -0,0 +1,7 @@"), with("+", "")), ""},
		{"too short", diff(header("a.go", "@@ -1,2 +1,2 @@"), []string{"-}", "-}"}, header("b.go", "@@ -1,2 +1,2 @@"), []string{"+}", "+}"}), ""},
	}
	o := &Optimizer{cfg: &Config{ContextLines: 1, MaxFileLines: 200, MinMoveLines: 4}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := o.moves(tt.lines)
			if tt.want == "" {
				if !slices.Equal(got, tt.lines) {
					t.Fatalf("changed:\n%s", strings.Join(got, "\n"))
				}
				return
			}
			if !slices.Contains(got, tt.want) || len(got) != len(tt.lines)-len(block)+1 {
				t.Fatalf("want %q in:\n%s", tt.want, strings.Join(got, "\n"))
			}
			if again := o.moves(got); !slices.Equal(again, got) {
				t.Fatal("not idempotent")
			}
		})
	}
}
