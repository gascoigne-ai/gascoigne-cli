package table

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func TestFoldReport(t *testing.T) {
	in := `== Fixtures
   Tests: text kept

 case          type       route    raw tok  out tok  saved       lost      ms  check
 go/test-pass  test/go    default      417       80  80.8%  ████████▏   0/3     1.1  ok
 go/test-fail  test/go    default      417      185  55.6%  █████▋      1/2     0.6  FAIL golden differs
 go/test-v     test/go    default     1404       72  94.9%  █████████▌  0/1     1.3  ok
 kubectl get pods -A                                                          skip: not installed
 total                                    2237      337  84.9%  ████████▍          3.0

Summary
  fixtures  FAIL
`
	want := `== Fixtures
   Tests: text kept

 case          type       route    raw tok  out tok  saved       lost      ms  check
 go/test-fail  test/go    default      417      185  55.6%  █████▋      1/2     0.6  FAIL golden differs
 kubectl get pods -A                                                          skip: not installed
 total                                    2237      337  84.9%  ████████▍          3.0
 … 2 ok

Summary
  fixtures  FAIL
`
	if got := compress.Join(FoldReport(compress.Lines(in))); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
