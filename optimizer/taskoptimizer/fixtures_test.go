package taskoptimizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

// Real tool output from testdata/eval run through the dialect handlers; the
// golden files pin the output so a refactor of a handler can be checked
// byte for byte.
func TestHandlersOnEvalFixtures(t *testing.T) {
	cases := []struct {
		name    string
		fixture string
		handler func([]string) []string
	}{
		{"nx-run-many-build", "nx/run-many-build.txt", nx},
		{"nx-run-many-test", "nx/run-many-test.txt", nx},
		{"gradle-fail", "task/gradle-fail.txt", gradle},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("..", "..", "testdata", "eval", tc.fixture))
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", tc.name+".golden"))
			if err != nil {
				t.Fatal(err)
			}
			got := compress.Join(tc.handler(compress.CleanLines(string(input))))
			if got != string(want) {
				t.Errorf("got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}
