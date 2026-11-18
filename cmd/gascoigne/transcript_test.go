package main

import (
	"bufio"
	"errors"
	"strings"
	"testing"
)

func TestScanTranscript(t *testing.T) {
	old := transcriptLineMax
	transcriptLineMax = 1024
	t.Cleanup(func() { transcriptLineMax = old })
	tests := []struct {
		name    string
		input   string
		want    []string
		wantErr error
	}{
		{"empty", "", nil, nil},
		{"lines", "a\nb\n", []string{"a", "b"}, nil},
		{"last line without newline", "a\nb", []string{"a", "b"}, nil},
		{"line over the cap stops the scan", "a\n" + strings.Repeat("x", 4096) + "\nb\n", []string{"a"}, bufio.ErrTooLong},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			err := scanTranscript(strings.NewReader(tc.input), func(line []byte) { got = append(got, string(line)) })
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want %v", err, tc.wantErr)
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Errorf("lines = %q, want %q", got, tc.want)
			}
		})
	}
}
