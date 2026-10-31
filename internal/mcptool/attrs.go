package mcptool

import (
	"cmp"
	"encoding/json"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Attrs gives the payload what a tool call (name and arguments) says about
// its output, so detection picks the right engine: the built-in tools of each
// agent are described as the shell command that prints the same thing.
// The LLM proxy and the PostToolUse hook both use it, so they type results alike.
func Attrs(name string, input map[string]any) map[string]string {
	a := map[string]string{payload.AttrTool: name}
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := input[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	if q := str("query", "q", "search", "prompt"); q != "" {
		a[payload.AttrQuery] = q // relevance for long pages, search results, JSON
	} else if q := sample.Intent(str("description")); q != "" {
		a[payload.AttrQuery] = q // what a shell call is for, in the agent's words
	}
	if u := str("url"); u != "" {
		a[payload.AttrURL] = u
	}
	switch strings.ToLower(name) {
	case "read", "read_file", "view", "view_file", "fs_read":
		f := str("file_path", "path", "absolute_path")
		if f == "" {
			break
		}
		a[payload.AttrFile] = f
		// A whole file reads like cat (gaps name file lines); a slice like
		// sed -n (gaps counted), since its first line is not line 1.
		a[payload.AttrCmd] = "cat " + f
		if offset, _ := input["offset"].(json.Number); offset != "" && offset != "0" && offset != "1" {
			a[payload.AttrCmd] = "sed -n " + string(offset) + ",$p " + f
		}
	case "grep", "grep_search", "search_file_content":
		args := "rg -n"
		if mode := str("output_mode"); mode == "files_with_matches" {
			args = "rg -l"
		} else if mode == "count" {
			args = "rg -c"
		}
		a[payload.AttrCmd] = args + " " + shellQuote(str("pattern", "query")) + " " + str("path")
	case "glob", "find", "list_dir", "ls", "list_directory":
		a[payload.AttrCmd] = "find " + cmp.Or(str("path"), ".") + " -name " + shellQuote(cmp.Or(str("pattern", "glob"), "*"))
	default:
		if cmd := str("command", "cmd"); cmd != "" {
			a[payload.AttrCmd] = cmd
		} else if f := str("file_path"); f != "" {
			a[payload.AttrFile] = f
			a[payload.AttrCmd] = "cat " + f
		}
	}
	a[payload.AttrCmd] = strings.TrimSpace(a[payload.AttrCmd])
	if a[payload.AttrCmd] == "" {
		delete(a, payload.AttrCmd)
	}
	return a
}

// shellQuote quotes s for display in a command line.
func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t'\"$`\\|&;<>()*?[]{}") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
