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

func TestKeepWithContext(t *testing.T) {
	in := strings.Split("a b c ERR d e f g", " ")
	got := KeepWithContext(in, func(line string) bool { return line == "ERR" }, 1, 1)
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

func TestOmittedSingular(t *testing.T) {
	for what, want := range map[string]string{"items": "… 1 item omitted", "more matches": "… 1 more match omitted", "cheap plan nodes": "… 1 cheap plan node omitted", "": "… 1 line omitted"} {
		if got := Omitted(1, what); got != want {
			t.Errorf("Omitted(1, %q) = %q, want %q", what, got, want)
		}
	}
}

// Split must agree with Normalize and Volatile, which it replaces on hot paths.
func TestSplitMatchesNormalizeAndVolatile(t *testing.T) {
	tests := []string{
		"",
		"plain words only",
		"2026-10-10T12:01:02.003Z INFO worker-3 request id=42 path=/api status=200 took=17ms",
		"uuid 123e4567-e89b-12d3-a456-426614174000 hash deadbeefcafe0123 ptr 0x7ffd1c",
		"7",
		"ends with number 3.14",
	}
	for _, in := range tests {
		t.Run(in, func(t *testing.T) {
			norm, vols := Split(in)
			if want := Normalize(in); norm != want {
				t.Errorf("norm = %q, want %q", norm, want)
			}
			want := Volatile(in)
			if len(vols) != len(want) {
				t.Fatalf("vols = %q, want %q", vols, want)
			}
			for i := range want {
				if vols[i] != want[i] {
					t.Errorf("vols[%d] = %q, want %q", i, vols[i], want[i])
				}
			}
		})
	}
}

func TestStripANSIKeepsPlainText(t *testing.T) {
	for in, want := range map[string]string{
		"plain\ntext":          "plain\ntext",
		"\x1b[31mred\x1b[0m x": "red x",
		"a\r\nb":               "a\nb",
	} {
		if got := StripANSI(in); got != want {
			t.Errorf("StripANSI(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCount(t *testing.T) {
	tests := []struct {
		count int
		word  string
		want  string
	}{
		{0, "test", "0 tests"},
		{1, "test", "1 test"},
		{2, "package", "2 packages"},
	}
	for _, tt := range tests {
		if got := Count(tt.count, tt.word); got != tt.want {
			t.Errorf("Count(%d, %q) = %q, want %q", tt.count, tt.word, got, tt.want)
		}
	}
}

func TestIsWord(t *testing.T) {
	for char := range 256 {
		want := char == '_' || char >= '0' && char <= '9' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z'
		if got := IsWord(byte(char)); got != want {
			t.Errorf("IsWord(%q) = %v, want %v", byte(char), got, want)
		}
	}
}

func TestStripEscapesKeepsCarriageReturns(t *testing.T) {
	if got := StripEscapes("\x1b[1;32mok\x1b[0m\r\n\x1b]0;title\x07x"); got != "ok\r\nx" {
		t.Fatalf("got %q", got)
	}
}

func TestHumanBytes(t *testing.T) {
	for _, c := range []struct {
		n     int64
		style ByteStyle
		want  string
	}{
		{0, LongBytes, "0 B"},
		{1023, LongBytes, "1023 B"},
		{1024, LongBytes, "1 KB"},
		{1536, LongBytes, "1.5 KB"},
		{1 << 20, LongBytes, "1 MB"},
		{5 << 40, LongBytes, "5 TB"},
		{2048 << 40, LongBytes, "2048 TB"},
		{0, ShortBytes, "0"},
		{1023, ShortBytes, "1023"},
		{1024, ShortBytes, "1K"},
		{1536, ShortBytes, "1.5K"},
		{3 << 30, ShortBytes, "3G"},
		{1023, WholeKB, "1023 B"},
		{1536, WholeKB, "1 KB"},
		{46*1024 + 600, WholeKB, "46 KB"},
		{5 << 20, WholeKB, "5120 KB"},
	} {
		if got := HumanBytes(c.n, c.style); got != c.want {
			t.Errorf("HumanBytes(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

func TestTally(t *testing.T) {
	tally := Tally{"b": 2, "a": 2, "c": 9}
	if got := tally.Counts(false); got != "a 2, b 2, c 9" {
		t.Errorf("Counts(false) = %q", got)
	}
	if got := tally.Counts(true); got != "c 9, a 2, b 2" {
		t.Errorf("Counts(true) = %q", got)
	}
	if got := tally.Top(2); got != "c ×9, a ×2, … 1 more" {
		t.Errorf("Top(2) = %q", got)
	}
	if got := tally.Top(5); got != "c ×9, a ×2, b ×2" {
		t.Errorf("Top(5) = %q", got)
	}
	if got := (Tally{}).Top(3); got != "" {
		t.Errorf("empty Top = %q", got)
	}
}

func TestIndent(t *testing.T) {
	if got := Indent([]string{"a", "", "b"}); !slices.Equal(got, []string{"  a", "  ", "  b"}) {
		t.Fatalf("Indent = %q", got)
	}
	if got := Indent(nil); len(got) != 0 {
		t.Fatalf("Indent(nil) = %q", got)
	}
}

func TestIsDigit(t *testing.T) {
	for char, want := range map[byte]bool{'0': true, '9': true, '5': true, '/': false, ':': false, 'a': false} {
		if IsDigit(char) != want {
			t.Errorf("IsDigit(%q) = %v", char, !want)
		}
	}
}

func TestStripHTMLComments(t *testing.T) {
	in := "a <!-- one --> b\n<!--\nmulti\nline\n-->c <!-- unclosed"
	if got, want := StripHTMLComments(in), "a  b\nc <!-- unclosed"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDedupeExact(t *testing.T) {
	in := []string{"a", "a", "b", "b", "b", "", "", "", "c"}
	want := []string{"a", "a", "b (×3)", "", "", "", "c"}
	if got := DedupeExact(in); !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
