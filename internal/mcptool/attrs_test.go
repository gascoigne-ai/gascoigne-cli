package mcptool

import (
	"encoding/json"
	"maps"
	"testing"
)

func TestAttrsCommandPerTool(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tool  string
		input map[string]any
		want  map[string]string
	}{
		{"read whole file", "Read", map[string]any{"file_path": "a.go"}, map[string]string{"tool": "Read", "file": "a.go", "cmd": "cat a.go"}},
		{"read path key, lower case tool", "read_file", map[string]any{"path": "b.go", "offset": json.Number("1")}, map[string]string{"tool": "read_file", "file": "b.go", "cmd": "cat b.go"}},
		{"read slice", "Read", map[string]any{"file_path": "a.go", "offset": json.Number("40")}, map[string]string{"tool": "Read", "file": "a.go", "cmd": "sed -n 40,$p a.go"}},
		{"read without file", "Read", map[string]any{}, map[string]string{"tool": "Read"}},
		{"grep default", "Grep", map[string]any{"pattern": "foo", "path": "src"}, map[string]string{"tool": "Grep", "cmd": "rg -n foo src"}},
		{"grep files", "Grep", map[string]any{"pattern": "a b", "output_mode": "files_with_matches"}, map[string]string{"tool": "Grep", "cmd": "rg -l 'a b'"}},
		{"grep count", "grep_search", map[string]any{"query": "x", "output_mode": "count"}, map[string]string{"tool": "grep_search", "query": "x", "cmd": "rg -c x"}},
		{"glob", "Glob", map[string]any{"pattern": "*.go", "path": "src"}, map[string]string{"tool": "Glob", "cmd": "find src -name '*.go'"}},
		{"glob defaults", "ls", map[string]any{}, map[string]string{"tool": "ls", "cmd": "find . -name '*'"}},
		{"shell command", "Bash", map[string]any{"command": "go test ./...", "description": "run the tests"}, map[string]string{"tool": "Bash", "cmd": "go test ./...", "query": "tests"}},
		{"shell cmd key", "run", map[string]any{"cmd": " ls "}, map[string]string{"tool": "run", "cmd": "ls"}},
		{"unknown tool with file", "edit", map[string]any{"file_path": "c.go"}, map[string]string{"tool": "edit", "file": "c.go", "cmd": "cat c.go"}},
		{"unknown tool, nothing", "mystery", map[string]any{"n": 1}, map[string]string{"tool": "mystery"}},
		{"url and query", "fetch", map[string]any{"url": "https://x.dev", "q": "price"}, map[string]string{"tool": "fetch", "url": "https://x.dev", "query": "price"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Attrs(tc.tool, tc.input)
			if !maps.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
