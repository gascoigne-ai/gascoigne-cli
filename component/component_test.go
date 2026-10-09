package component

import "testing"

func TestParseID(t *testing.T) {
	tests := []struct {
		in      string
		want    ID
		wantErr bool
	}{
		{"foo", ID{"foo", ""}, false},
		{"foo/bar", ID{"foo", "bar"}, false},
		{"", ID{}, true},
		{"/bar", ID{}, true},
		{"a/b/c", ID{}, true},
	}
	for _, tt := range tests {
		got, err := ParseID(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseID(%q) = %v, %v", tt.in, got, err)
		}
		if err == nil && got.String() != tt.in {
			t.Errorf("String() = %q, want %q", got.String(), tt.in)
		}
	}
}
