package gooptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func TestSummarize(t *testing.T) {
	in := `=== RUN   TestA
    a_test.go:5: noise from passing test
--- PASS: TestA (0.00s)
=== RUN   TestB
    b_test.go:9: want 1, got 2
--- FAIL: TestB (0.01s)
FAIL
FAIL	example.com/x	0.02s
ok  	example.com/y	0.01s
ok  	example.com/z	0.01s
?   	example.com/w	[no test files]
`
	got := compress.Join(summarize(compress.Lines(in)))
	want := `--- FAIL: TestB (0.01s)
    b_test.go:9: want 1, got 2
FAIL	example.com/x	0.02s
ok 2 packages in example.com: y, z
1 package without tests
1 test passed
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestPanicKeepsUserFrames(t *testing.T) {
	in := `--- FAIL: TestX (0.00s)
panic: boom [recovered]

goroutine 7 [running]:
testing.tRunner.func1.2({0x1, 0x2})
	/usr/local/go/src/testing/testing.go:1974 +0x232
panic({0x1?, 0x2?})
	/usr/local/go/src/runtime/panic.go:860 +0x13a
example.com/x.TestX(0x1?)
	/src/x/x_test.go:7 +0x28
created by testing.(*T).Run in goroutine 1
	/usr/local/go/src/testing/testing.go:2101 +0x4c5
FAIL	example.com/x	0.003s
`
	got := compress.Join(summarize(compress.Lines(in)))
	for _, keep := range []string{"panic: boom", "example.com/x.TestX", "/src/x/x_test.go:7", "… 3 standard library frames omitted", "FAIL\texample.com/x"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q:\n%s", keep, got)
		}
	}
	if strings.Contains(got, "testing.go") || strings.Contains(got, "panic.go") {
		t.Errorf("kept std frames:\n%s", got)
	}
}

func TestIsStdFrame(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"testing.tRunner(0x1, 0x2)", true},
		{"panic({0x1, 0x2})", true},
		{"created by testing.(*T).Run in goroutine 1", true},
		{"net/http.(*conn).serve(0x1)", true},
		{"example.com/x.TestX(0x1)", false},
		{"github.com/a/b.(*T).Run(0x1)", false},
	}
	for _, tt := range tests {
		if got := isStdFrame(tt.line); got != tt.want {
			t.Errorf("isStdFrame(%q) = %v", tt.line, got)
		}
	}
}

func TestOptimizeJSON(t *testing.T) {
	events := `{"Action":"start","Package":"example.com/x"}
{"Action":"run","Package":"example.com/x","Test":"TestA"}
{"Action":"output","Package":"example.com/x","Test":"TestA","Output":"=== RUN   TestA\n"}
{"Action":"output","Package":"example.com/x","Test":"TestA","Output":"    a_test.go:3: bad\n"}
{"Action":"output","Package":"example.com/x","Test":"TestA","Output":"--- FAIL: TestA (0.00s)\n"}
{"Action":"fail","Package":"example.com/x","Test":"TestA"}
{"Action":"fail","Package":"example.com/x"}
{"Action":"pass","Package":"example.com/y"}
`
	p := &payload.Payload{Blocks: []payload.Block{{Text: events, Type: detect.GoTestJSON}, {Text: "untouched", Type: detect.Log}}}
	if err := (Optimizer{}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	want := "--- FAIL: TestA (0.00s)\n    a_test.go:3: bad\nFAIL\texample.com/x\nok 1 package: example.com/y\n"
	if p.Blocks[0].Text != want || p.Blocks[1].Text != "untouched" {
		t.Fatalf("got:\n%q", p.Blocks[0].Text)
	}
}
