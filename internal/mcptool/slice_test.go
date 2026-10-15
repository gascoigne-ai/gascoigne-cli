package mcptool

import "testing"

func TestSlice(t *testing.T) {
	text := "alpha\nbeta\ngamma\ndelta FAIL\nepsilon\nzeta FAIL timeout\neta\n"
	tests := []struct {
		name string
		s    Slice
		want string
	}{
		{"whole", Slice{}, text},
		{"range", Slice{Lines: "2-3"}, "beta\ngamma\n"},
		{"single line", Slice{Lines: "4"}, "delta FAIL\n"},
		{"open end", Slice{Lines: "6-"}, "zeta FAIL timeout\neta\n"},
		{"open start", Slice{Lines: "-1"}, "alpha\n"},
		{"past end clamped", Slice{Lines: "7-99"}, "eta\n"},
		{"grep", Slice{Grep: "FAIL"}, "4:delta FAIL\n--\n6:zeta FAIL timeout\n"},
		{"grep context merges runs", Slice{Grep: "FAIL", Context: 1}, "3-gamma\n4:delta FAIL\n5-epsilon\n6:zeta FAIL timeout\n7-eta\n"},
		{"grep none", Slice{Grep: "panic"}, "no matching lines\n"},
		{"query ranks", Slice{Query: "timeout"}, "6:zeta FAIL timeout\n"},
		{"query max", Slice{Query: "fail", Max: 1}, "4:delta FAIL\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.s.Apply(text)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSliceErrors(t *testing.T) {
	for _, s := range []Slice{{Lines: "x"}, {Lines: "5-2"}, {Lines: "9-10"}, {Grep: "("}} {
		if _, err := s.Apply("a\nb\n"); err == nil {
			t.Errorf("%+v: want error", s)
		}
	}
}
