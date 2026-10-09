package detect

import (
	"path"
	"regexp"
	"strings"
)

// Languages, grouped by how their blocks are delimited.
const (
	LangGo     = "go"
	LangPython = "python"
	LangShell  = "shell"
	LangCLike  = "c-like" // brace-delimited: js, ts, java, c, c++, c#, rust, php, kotlin, swift…
)

var langByExt = map[string]string{
	".go": LangGo,
	".py": LangPython, ".pyi": LangPython,
	".sh": LangShell, ".bash": LangShell, ".zsh": LangShell,
	".js": LangCLike, ".mjs": LangCLike, ".cjs": LangCLike, ".jsx": LangCLike,
	".ts": LangCLike, ".tsx": LangCLike, ".mts": LangCLike,
	".java": LangCLike, ".kt": LangCLike, ".scala": LangCLike, ".swift": LangCLike,
	".c": LangCLike, ".h": LangCLike, ".cc": LangCLike, ".cpp": LangCLike, ".hpp": LangCLike,
	".cs": LangCLike, ".rs": LangCLike, ".php": LangCLike, ".dart": LangCLike,
}

var langSniffers = []struct {
	lang string
	re   *regexp.Regexp
}{
	{LangGo, regexp.MustCompile(`(?m)^package \w+\s*$`)},
	{LangPython, regexp.MustCompile(`(?m)^(def \w+\(.*\):|class \w+(\(.*\))?:|from [\w.]+ import |import \w+\s*$)`)},
	{LangShell, regexp.MustCompile(`^#!.*\b(ba|z)?sh\b`)},
	{LangCLike, regexp.MustCompile(`(?m)^(import .* from |export (default |const |function |class |interface )|#include [<"]|(public |private )?(static )?(class|interface|enum) \w+.*\{|fn \w+\(|use [\w:]+;|function \w+\()`)},
}

// Lang returns the language of source code, or "" when text is not code.
func Lang(text string, h Hints) string {
	file := h.File
	if file == "" {
		if args := Argv(h.Cmd); len(args) > 1 {
			file = args[len(args)-1]
		}
	}
	if l, ok := langByExt[strings.ToLower(path.Ext(file))]; ok {
		return l
	}
	head := text[:min(len(text), 8192)]
	for _, s := range langSniffers {
		if s.re.MatchString(head) {
			return s.lang
		}
	}
	return ""
}
