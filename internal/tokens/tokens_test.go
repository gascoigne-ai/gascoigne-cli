package tokens

import "testing"

func TestEstimate(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"abcde", 2},
	}
	for _, tt := range tests {
		if got := Estimate(tt.in); got != tt.want {
			t.Errorf("Estimate(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
