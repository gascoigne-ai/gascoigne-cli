package testreport

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
)

func TestCompressDialects(t *testing.T) {
	tests := []struct {
		dialect, in string
		keep, drop  []string
	}{
		{"pytest", "platform linux -- Python 3.13\nrootdir: /src\ntests/test_a.py ........ [ 50%]\ntests/test_b.py F [100%]\n=== FAILURES ===\nE assert 1 == 2\n=== 1 failed, 8 passed in 0.1s ===\n",
			[]string{"tests/test_b.py F", "E assert 1 == 2", "1 failed, 8 passed", "1 passing test line"}, []string{"rootdir", "test_a.py ...."}},
		{"cargo", "   Compiling x v0.1.0\nrunning 3 tests\ntest a ... ok\ntest b ... FAILED\nfailures:\nleft: 1\ntest result: FAILED. 1 passed; 1 failed\n",
			[]string{"test b ... FAILED", "left: 1", "test result: FAILED"}, []string{"Compiling", "test a ... ok"}},
		{"maven", "[INFO] Scanning for projects...\nDownloading from central: https://repo/x.jar\n[ERROR] Tests run: 3, Failures: 1\n\tat org.junit.jupiter.api.Assertions.assertEquals(Assertions.java:1)\n\tat com.x.T.t(T.java:9)\n[INFO] BUILD FAILURE\n",
			[]string{"Tests run: 3, Failures: 1", "T.java:9", "BUILD FAILURE", "dependency stack frame"}, []string{"Downloading", "Assertions.java"}},
		{"deno", "Check a_test.ts\nx ... ok (12µs)\ny ... FAILED (3ms)\nFAILED | 1 passed | 1 failed (5ms)\n",
			[]string{"y ... FAILED", "FAILED | 1 passed"}, []string{"Check", "x ... ok"}},
		{"vitest", " RUN  v3.2.7 /src\n ✓ a.test.mjs (8 tests) 2ms\n × b > c 1ms\n⎯⎯⎯⎯⎯ Failed Tests 1 ⎯⎯⎯⎯⎯\n⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯[1/1]⎯\n Tests  1 failed | 8 passed (9)\n",
			[]string{"× b > c", "Failed Tests 1", "Tests  1 failed"}, []string{"RUN", "a.test.mjs (8 tests)", "[1/1]"}},
	}
	for _, tt := range tests {
		t.Run(tt.dialect, func(t *testing.T) {
			out, ok := Compress(tt.dialect, compress.Lines(tt.in))
			got := compress.Join(out)
			if !ok {
				t.Fatal("dialect unknown")
			}
			for _, k := range tt.keep {
				if !strings.Contains(got, k) {
					t.Errorf("lost %q in:\n%s", k, got)
				}
			}
			for _, d := range tt.drop {
				if strings.Contains(got, d) {
					t.Errorf("kept %q in:\n%s", d, got)
				}
			}
		})
	}
	if _, ok := Compress("nope", nil); ok {
		t.Error("unknown dialect accepted")
	}
}
