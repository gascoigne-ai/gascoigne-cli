package jsonx

import (
	"strings"
	"testing"
)

func TestSpans(t *testing.T) {
	tests := []struct {
		name, text string
		want       []string
	}{
		{"labelled object", `body: {"a":[1,2]} done`, []string{`{"a":[1,2]}`}},
		{"pretty block", "resp:\n{\n  \"a\": \"}\"\n}\nnext", []string{"{\n  \"a\": \"}\"\n}"}},
		{"two values", `x=[1] y={"b":2}`, []string{`[1]`, `{"b":2}`}},
		{"log level brackets", `[INFO] started [worker-1]`, nil},
		{"top level only", `{"a":{"b":[1]}}`, []string{`{"a":{"b":[1]}}`}},
		{"unbalanced", `{"a": [1, 2}`, nil},
		{"bad inside good", `[x] {"ok":true}`, []string{`{"ok":true}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, s := range Spans(tt.text) {
				got = append(got, tt.text[s[0]:s[1]])
			}
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSpansBounded(t *testing.T) {
	text := strings.Repeat("{", 200000) // every start walks to the end
	if got := Spans(text); got != nil {
		t.Fatalf("got %v", got)
	}
}
