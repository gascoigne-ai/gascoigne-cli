package prose

import "testing"

func TestProgressLine(t *testing.T) {
	const mb = 1 << 20
	tests := []struct {
		name        string
		total, done int64
		want        string
	}{
		{"half", 100 * mb, 50 * mb, "[###############---------------]  50%  50/100 MB"},
		{"done", 10 * mb, 10 * mb, "[##############################] 100%  10/10 MB"},
		{"unknown size", -1, 7 * mb, "7 MB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &progress{total: tt.total, done: tt.done}
			if got := p.line(); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
