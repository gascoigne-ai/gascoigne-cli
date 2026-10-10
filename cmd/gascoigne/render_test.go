package main

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHuman(t *testing.T) {
	for in, want := range map[int]string{0: "0", 950: "950", 1000: "1k", 1234: "1.2k", 47399: "47.4k", 1_300_000: "1.3M"} {
		if got := human(in); got != want {
			t.Errorf("human(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestBar(t *testing.T) {
	tests := []struct {
		frac float64
		want string
	}{
		{0, "    "},
		{1, "████"},
		{0.5, "██  "},
		{0.53125, "██▏ "}, // 17 of 32 eighths
		{2, "████"},
	}
	for _, tt := range tests {
		if got := bar(tt.frac, 4); got != tt.want {
			t.Errorf("bar(%v) = %q, want %q", tt.frac, got, tt.want)
		}
	}
}

var ansi = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func TestTableAlignsWithColors(t *testing.T) {
	var plain, colored strings.Builder
	rows := [][]cell{
		{{text: "go test"}, {text: "12", right: true, style: green}},
		{{text: "ls"}, {text: "3", right: true, style: red}},
	}
	painter{false}.table(&plain, []string{"command", "runs"}, rows)
	painter{true}.table(&colored, []string{"command", "runs"}, rows)
	if got := ansi.ReplaceAllString(colored.String(), ""); got != plain.String() {
		t.Fatalf("colors change layout:\n%s\nvs\n%s", got, plain.String())
	}
	if !strings.Contains(colored.String(), "\x1b[32m12\x1b[0m") {
		t.Fatalf("no color codes: %q", colored.String())
	}
	lines := strings.Split(strings.TrimSpace(plain.String()), "\n")
	if utf8.RuneCountInString(lines[1]) != utf8.RuneCountInString(lines[2]) {
		t.Fatalf("rows not aligned:\n%s", plain.String())
	}
}

func TestNewPainter(t *testing.T) {
	if p, _ := newPainter("always"); !p.color {
		t.Error("always: no color")
	}
	if p, _ := newPainter("auto"); p.color {
		t.Error("auto: color without a terminal")
	}
	if _, err := newPainter("rainbow"); err == nil {
		t.Error("bad mode accepted")
	}
}
