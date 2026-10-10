package tokens

import (
	"strings"
	"testing"
)

func TestEstimate(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"hello world", 2},
		{"reconciliation", 2},
		{"        ", 1}, // indentation is one token
		{"\n\n\n", 1},
		{"=====", 1},
		{"0123456789", 4},
		{"x := 42", 4},
		{"┌──────┐", 3},
		{"\x00\x01", 2}, // control bytes are pieces too
	}
	for _, tt := range tests {
		if got := Estimate(tt.in); got != tt.want {
			t.Errorf("Estimate(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestEstimateWhitespaceIsCheap(t *testing.T) {
	code := "func f() {\n\treturn 1\n}\n"
	indented := strings.ReplaceAll(code, "\t", "        ")
	if a, b := Estimate(code), Estimate(indented); a != b {
		t.Fatalf("indentation width changed the estimate: %d vs %d", a, b)
	}
}

func BenchmarkEstimate(b *testing.B) {
	s := strings.Repeat("2026-10-10T07:00:01Z INFO GET /api/products/42 status=200 dur=12ms\n", 2000)
	b.SetBytes(int64(len(s)))
	for b.Loop() {
		Estimate(s)
	}
}
