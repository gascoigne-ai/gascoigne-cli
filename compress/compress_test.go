package compress

import (
	"slices"
	"strings"
	"testing"
)

func TestStripANSI(t *testing.T) {
	if got := StripANSI("\x1b[1;32mok\x1b[0m\r"); got != "ok" {
		t.Fatalf("got %q", got)
	}
}

func TestLinesJoin(t *testing.T) {
	if got := Lines("a\nb\n"); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("Lines = %q", got)
	}
	if Lines("") != nil || Join(nil) != "" {
		t.Fatal("empty round trip")
	}
	if got := Join([]string{"a", "b"}); got != "a\nb\n" {
		t.Fatalf("Join = %q", got)
	}
}

func TestDedupeRuns(t *testing.T) {
	in := []string{
		"2026-01-01T10:00:00Z retry 1 of 5",
		"2026-01-01T10:00:01Z retry 2 of 5",
		"2026-01-01T10:00:02Z retry 3 of 5",
		"done",
		"done",
		"x",
	}
	in = append(in, "", "")
	want := []string{"2026-01-01T10:00:00Z retry 1 of 5 (×3)", "done (×2)", "x", ""}
	if got := DedupeRuns(in); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestDedupeAll(t *testing.T) {
	in := []string{"a 1", "b", "a 2", "c", "a 3"}
	want := []string{"a 1 (×3)", "b", "c"}
	if got := DedupeAll(in); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestKeepWithContext(t *testing.T) {
	in := strings.Split("a b c ERR d e f g", " ")
	got := KeepWithContext(in, func(l string) bool { return l == "ERR" }, 1, 1)
	want := []string{"… 2 lines omitted", "c", "ERR", "d", "… 3 lines omitted"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestHeadTail(t *testing.T) {
	in := strings.Split("1 2 3 4 5 6 7", " ")
	if got := HeadTail(in, 2, 1); !slices.Equal(got, []string{"1", "2", "… 4 lines omitted", "7"}) {
		t.Fatalf("got %q", got)
	}
	if got := HeadTail(in[:4], 2, 1); len(got) != 4 {
		t.Fatalf("short input changed: %q", got)
	}
}

func TestTruncateLine(t *testing.T) {
	if got := TruncateLine("abcdefghij", 4); got != "ab … ij" {
		t.Fatalf("got %q", got)
	}
	if got := TruncateLine("abc", 4); got != "abc" {
		t.Fatalf("got %q", got)
	}
}

func TestSqueezeBlank(t *testing.T) {
	got := SqueezeBlank([]string{"a  ", "", "", "", "b"})
	if !slices.Equal(got, []string{"a", "", "b"}) {
		t.Fatalf("got %q", got)
	}
}

func TestSeverity(t *testing.T) {
	tests := []struct {
		line string
		want Severity
	}{
		{"npm ERR! code E404", Error},
		{"panic: runtime error", Error},
		{"--- FAIL: TestX", Error},
		{"npm warn deprecated glob@7", Warn},
		{"all good", None},
		{"terrorist", None},
	}
	for _, tt := range tests {
		if got := SeverityOf(tt.line); got != tt.want {
			t.Errorf("SeverityOf(%q) = %v, want %v", tt.line, got, tt.want)
		}
	}
}

func TestJSTest(t *testing.T) {
	in := Lines(`PASS src/a.test.ts
PASS src/c.test.ts
FAIL src/b.test.ts
  ● math › adds

    expect(received).toBe(expected)

    Expected: 3
    Received: 2

      at Object.<anonymous> (src/b.test.ts:4:17)
      at Promise.then.completed (node_modules/jest-circus/build/utils.js:298:28)
      at new Promise (<anonymous>)


Tests:       1 failed, 7 passed, 8 total
`)
	got := Join(JSTest(in))
	for _, keep := range []string{"FAIL src/b.test.ts", "● math › adds", "Received: 2", "src/b.test.ts:4:17", "Tests:       1 failed"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q in:\n%s", keep, got)
		}
	}
	for _, drop := range []string{"PASS src/a.test.ts", "node_modules/jest-circus"} {
		if strings.Contains(got, drop) {
			t.Errorf("kept %q in:\n%s", drop, got)
		}
	}
	if !strings.Contains(got, "… 2 passing test lines omitted") {
		t.Errorf("missing pass marker:\n%s", got)
	}
}
