package diag

import (
	"strconv"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func run(in string, r Rules) string { return compress.Join(Compress(compress.Lines(in), r)) }

func TestGroupsByFileAndDedupes(t *testing.T) {
	in := `src/a.ts(3,9): error TS2322: Type 'string' is not assignable to type 'number'.
src/a.ts(9,1): error TS7006: Parameter 'u' implicitly has an 'any' type.
src/b.ts(3,9): error TS2322: Type 'string' is not assignable to type 'number'.
Found 3 errors in 2 files.
`
	got := run(in, Rules{})
	want := `src/a.ts
  3:9 error TS2322 Type 'string' is not assignable to type 'number'.
  9:1 error TS7006 Parameter 'u' implicitly has an 'any' type.
src/b.ts
  3:9 error TS2322 Type 'string' is not assignable to type 'number'.
Found 3 errors in 2 files.
3 errors in 2 files
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSharedAcrossFiles(t *testing.T) {
	var b strings.Builder
	for _, f := range []string{"a", "b", "c", "d"} {
		b.WriteString(f + ".c:6:5: error: 'x' undeclared\n")
	}
	got := run(b.String(), Rules{})
	if !strings.Contains(got, "same in several files:") || !strings.Contains(got, "at a.c:6, b.c:6, c.c:6, d.c:6") || strings.Count(got, "'x' undeclared") != 1 {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRustAndRuffHeaders(t *testing.T) {
	in := `error[E0308]: mismatched types
  --> src/cart.rs:12:5
   |
10 | pub fn total() -> u64 {
   |                   --- expected because of return type
11 |     let s: i64 = 1;
12 |     s
   |     ^ expected u64, found i64
F401 [*] ` + "`os`" + ` imported but unused
 --> a.py:1:8
error: could not compile ` + "`shop`" + `
`
	got := run(in, Rules{MaxFrame: 2})
	for _, want := range []string{"src/cart.rs", "12:5 error E0308 mismatched types", "^ expected u64, found i64", "a.py", "1:8 F401", "could not compile"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestNoiseAndStrip(t *testing.T) {
	in := "#8 1.234 npm http fetch GET 200 x\n#10 3.9 src/a.ts(1,1): error TS1: boom\n#3 DONE 0.0s\n"
	got := run(in, For("docker"))
	if strings.Contains(got, "DONE") || !strings.Contains(got, "1:1 error TS1 boom") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestMaven(t *testing.T) {
	got := run("[ERROR] /src/Cart.java:[14,20] incompatible types\n", Rules{})
	if !strings.Contains(got, "/src/Cart.java\n  14:20 error incompatible types") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestSARIF(t *testing.T) {
	log := `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"semgrep","rules":[{"id":"r1"}]}},"results":[
 {"ruleId":"r1","level":"error","message":{"text":"tainted SQL\nmore help"},"locations":[{"physicalLocation":{"artifactLocation":{"uri":"file://app/db.py"},"region":{"startLine":12,"startColumn":4}}}]},
 {"ruleId":"r2","message":{"text":"no location"}}]}]}`
	got, err := SARIF(log)
	if err != nil {
		t.Fatal(err)
	}
	want := "semgrep: 2 results\napp/db.py:12:4: error: tainted SQL [r1]\nwarning: no location [r2]"
	if strings.Join(got, "\n") != want {
		t.Fatalf("got\n%s\nwant\n%s", strings.Join(got, "\n"), want)
	}
}

func TestMavenDetailsStayWithTheirError(t *testing.T) {
	in := `[ERROR] /src/T.java:[52,51] cannot find symbol
[ERROR]   symbol:   variable Attrs
[ERROR]   location: class T
[ERROR] /src/T.java:[53,51] cannot find symbol
[ERROR]   symbol:   variable Attrs
[ERROR]   location: class T
[ERROR] /src/T.java:[60,3] cannot find symbol
[ERROR]   symbol:   variable Other
[ERROR]   location: class T
`
	want := `/src/T.java
  52:51 error cannot find symbol [symbol: variable Attrs; location: class T] (×2, also at 53)
  60:3 error cannot find symbol [symbol: variable Other; location: class T]
3 errors in 1 file
`
	if got := run(in, Rules{}); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRepeatsListTheirLines(t *testing.T) {
	var in strings.Builder
	for i := 1; i <= 15; i++ {
		in.WriteString("a.go:" + strconv.Itoa(i*10) + ":1: error: boom\n")
	}
	got := run(in.String(), Rules{})
	if !strings.Contains(got, "(×15, also at 20, 30, 40, 50, 60, 70, 80, 90, 100, 110, 120, … 3 more)") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestRepeatOnTheSameLine(t *testing.T) {
	got := run("a.go:4:1: error: boom\na.go:4:1: error: boom\n", Rules{})
	if !strings.Contains(got, "4:1 error boom (×2, same place)") {
		t.Fatalf("got:\n%s", got)
	}
}

// Every noise rule must match its own examples and none of its near misses.
func TestNoiseRulesVerify(t *testing.T) {
	for _, d := range Dialects() {
		t.Run(d, func(t *testing.T) {
			if err := For(d).Noise.Verify(); err != nil {
				t.Error(err)
			}
		})
	}
}
