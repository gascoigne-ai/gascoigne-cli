package detect

import (
	"path"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Languages, grouped by how their blocks are delimited.
const (
	LangGo     = "go"
	LangPython = "python"
	LangShell  = "shell"
	LangCLike  = "c-like" // brace-delimited: js, ts, java, c, c++, c#, rust, php, kotlin, swift…
	LangOther  = "other"  // known source without an outline strategy: ruby, lua, sql, lisp…
)

// Language is one row of the language table: how to find it and how its
// comments and multi-line strings look. Adding a language is adding a row.
type Language struct {
	Name  string
	Class string   // one of the Lang* constants: picks the outline strategy
	Exts  []string // lower-case, with the dot

	// Comment syntax. A row without Line and Block takes its class's default.
	Line       string    // line comment token
	Block      [2]string // block comment delimiters; empty when none
	Strs       []string  // delimiters of strings that can span lines
	Heredoc    bool      // <<EOF … EOF (shell, ruby, perl, php)
	Docstrings bool      // python: a string right after def/class is documentation
}

// HasSyntax reports whether the row spells out its comment syntax.
func (l Language) HasSyntax() bool { return l.Line != "" || l.Block[0] != "" }

var (
	slashBlock = [2]string{"/*", "*/"}
	tripleStrs = []string{`"""`, `'''`}
)

var languages = []Language{
	{Name: "go", Class: LangGo, Exts: []string{".go"}},
	{Name: "python", Class: LangPython, Exts: []string{".py", ".pyi"}},
	{Name: "shell", Class: LangShell, Exts: []string{".sh", ".bash", ".zsh"}},

	{Name: "javascript", Class: LangCLike, Exts: []string{".js", ".mjs", ".cjs", ".jsx"}},
	{Name: "typescript", Class: LangCLike, Exts: []string{".ts", ".tsx", ".mts"}},
	{Name: "java", Class: LangCLike, Exts: []string{".java"}},
	{Name: "kotlin", Class: LangCLike, Exts: []string{".kt"}},
	{Name: "scala", Class: LangCLike, Exts: []string{".scala"}},
	{Name: "swift", Class: LangCLike, Exts: []string{".swift"}},
	{Name: "c", Class: LangCLike, Exts: []string{".c", ".h"}},
	{Name: "c++", Class: LangCLike, Exts: []string{".cc", ".cpp", ".hpp"}},
	{Name: "c#", Class: LangCLike, Exts: []string{".cs"}},
	{Name: "rust", Class: LangCLike, Exts: []string{".rs"}},
	{Name: "php", Class: LangCLike, Exts: []string{".php"}, Line: "//", Block: slashBlock, Strs: []string{"`", `"""`}, Heredoc: true},
	{Name: "dart", Class: LangCLike, Exts: []string{".dart"}},
	{Name: "zig", Class: LangCLike, Exts: []string{".zig"}},

	{Name: "ruby", Class: LangOther, Exts: []string{".rb"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "perl", Class: LangOther, Exts: []string{".pl", ".pm"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "r", Class: LangOther, Exts: []string{".r"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "julia", Class: LangOther, Exts: []string{".jl"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "nim", Class: LangOther, Exts: []string{".nim"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "elixir", Class: LangOther, Exts: []string{".ex", ".exs"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "terraform", Class: LangOther, Exts: []string{".tf"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "powershell", Class: LangOther, Exts: []string{".ps1"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "lua", Class: LangOther, Exts: []string{".lua"}, Line: "--"},
	{Name: "haskell", Class: LangOther, Exts: []string{".hs"}, Line: "--"},
	{Name: "elm", Class: LangOther, Exts: []string{".elm"}, Line: "--"},
	{Name: "erlang", Class: LangOther, Exts: []string{".erl", ".hrl"}, Line: "%"},
	{Name: "clojure", Class: LangOther, Exts: []string{".clj", ".cljs"}, Line: ";"},
	{Name: "emacs-lisp", Class: LangOther, Exts: []string{".el"}, Line: ";"},
	{Name: "lisp", Class: LangOther, Exts: []string{".lisp"}, Line: ";"},
	{Name: "scheme", Class: LangOther, Exts: []string{".scm"}, Line: ";"},

	// Config files read to be edited (see the detect filter's edit_safe).
	{Name: "yaml", Class: LangOther, Exts: []string{".yaml", ".yml"}, Line: "#"},
	{Name: "toml", Class: LangOther, Exts: []string{".toml"}, Line: "#"},
	{Name: "ini", Class: LangOther, Exts: []string{".ini"}, Line: ";"},
	{Name: "conf", Class: LangOther, Exts: []string{".conf"}, Line: "#"},
}

var languageByExt = func() map[string]Language {
	m := map[string]Language{}
	for _, l := range languages {
		for _, e := range l.Exts {
			m[e] = l
		}
	}
	return m
}()

// LanguageOf is the language of the file named by the hints, by extension.
func LanguageOf(h Hints) (Language, bool) {
	l, ok := languageByExt[SourceExt(h)]
	return l, ok
}

var langSniffers = []struct {
	lang string
	re   *lazyre.Regexp
}{
	{LangGo, lazyre.New(`(?m)^package \w+\s*$`)},
	{LangPython, lazyre.New(`(?m)^(def \w+\(.*\):|class \w+(\(.*\))?:|from [\w.]+ import |import \w+\s*$)`)},
	{LangShell, lazyre.New(`^#!.*\b(ba|z)?sh\b`)},
	{LangCLike, lazyre.New(`(?m)^(import .* from |export (default |const |function |class |interface )|#include [<"]|(public |private )?(static )?(class|interface|enum) \w+.*\{|fn \w+\(|use [\w:]+;|function \w+\()`)},
}

// SourceExt is the lower-case extension of the file named by the file hint
// or the command's last argument (cat x.go), "" when there is none.
func SourceExt(h Hints) string {
	file := h.File
	if file == "" {
		if args := Argv(h.Cmd); len(args) > 1 {
			file = args[len(args)-1]
		}
	}
	return strings.ToLower(path.Ext(file))
}

// Lang returns the language class of source code, or "" when text is not code.
func Lang(text string, h Hints) string {
	if l, ok := LanguageOf(h); ok {
		return l.Class
	}
	head := text[:min(len(text), 8192)]
	for _, s := range langSniffers {
		if s.re.MatchString(head) {
			return s.lang
		}
	}
	return ""
}
