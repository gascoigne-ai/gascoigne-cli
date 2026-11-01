package gitoptimizer

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// git log --graph in the default format prefixes every line with graph
// columns, so no line started with "commit " and the log passed through whole.
func TestLogGraph(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"linear", `* commit 9616bf3a6daa69f3e0d30fbe83f1320785a79337 (HEAD -> master)
| Author: Dev <dev@example.com>
| Date:   2026-11-01
|
|     feat(mcp-proxy): trim tool descriptions
|
|     Task: .harness/tasks/036-mcp-tool-desc-trim.md
|
* commit 9e6c5e150d4cdd06412a86a90343d5bcd11c0e8e
| Author: Dev <dev@example.com>
| Date:   2026-11-01
|
|     feat(mcp): fetch session cache
|
|     Task: .harness/tasks/035-fetch-session-cache.md
|
`, "9616bf3 (HEAD -> master) 2026-11-01 Dev: feat(mcp-proxy): trim tool descriptions (+1 body lines)\n" +
			"9e6c5e1 2026-11-01 Dev: feat(mcp): fetch session cache (+1 body lines)\n"},
		{"merge", `*   commit 89abcdef0123456789abcdef0123456789abcdef
|\  Merge: 0123456 1234567
| | Author: Bob <bob@example.com>
| | Date:   2026-07-10
| |
| |     Merge branch 'x'
| |
| * commit 1234567890abcdef1234567890abcdef12345678
|/  Author: Ada <ada@example.com>
|   Date:   2026-07-09
|
|       fix: parser
|
`, "89abcde 2026-07-10 Bob: Merge branch 'x' [merge]\n1234567 2026-07-09 Ada: fix: parser\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(t, detect.GitLog, tt.in); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}
