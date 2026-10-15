package detect

import (
	"path"
	"slices"
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

	// Lexical quirks the brace outline must know to count braces right.
	NestedBlock bool      // Block comments nest (rust, scala)
	Nested      [2]string // a second, always nesting comment pair (d: /+ +/)
	Raw         string    // raw string flavour: RawRust r#"…"#, RawCpp R"x(…)x"
	Lifetimes   bool      // 'a is a lifetime, not the start of a char literal
	DigitSep    bool      // 1'000 digit separators

	// Outline strategy for languages that are not brace-delimited (see the
	// Style constants). Func matches the line a function starts on, Container
	// one that holds functions (class, module); Open matches every line that
	// opens an end-closed block.
	Style     string
	Open      string
	Func      string
	Container string
}

// Outline styles besides the brace outline of c-like languages.
const (
	StyleEnd   = "end"   // blocks close with end: ruby, elixir, lua, julia
	StyleDecl  = "decl"  // a declaration owns the deeper-indented lines under it: haskell, ocaml, nim
	StyleParen = "paren" // top-level forms close by parenthesis balance: lisps
)

// Raw string flavours.
const (
	RawRust = "rust"
	RawCpp  = "cpp"
)

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
	{Name: "kotlin", Class: LangCLike, Exts: []string{".kt", ".kts"}},
	{Name: "scala", Class: LangCLike, Exts: []string{".scala", ".sc", ".sbt"}, Line: "//", Block: slashBlock, Strs: []string{`"""`}, NestedBlock: true},
	{Name: "swift", Class: LangCLike, Exts: []string{".swift"}},
	{Name: "c", Class: LangCLike, Exts: []string{".c", ".h"}},
	{Name: "c++", Class: LangCLike, Exts: []string{".cc", ".cpp", ".hpp", ".cxx", ".hh", ".hxx", ".ipp", ".tpp", ".inl", ".cppm", ".ixx", ".cu", ".cuh"}, Line: "//", Block: slashBlock, Raw: RawCpp, DigitSep: true},
	{Name: "c#", Class: LangCLike, Exts: []string{".cs"}},
	{Name: "rust", Class: LangCLike, Exts: []string{".rs"}, Line: "//", Block: slashBlock, NestedBlock: true, Raw: RawRust, Lifetimes: true},
	{Name: "d", Class: LangCLike, Exts: []string{".d", ".di"}, Line: "//", Block: slashBlock, Nested: [2]string{"/+", "+/"}, Strs: []string{"`"}},
	{Name: "php", Class: LangCLike, Exts: []string{".php"}, Line: "//", Block: slashBlock, Strs: []string{"`", `"""`}, Heredoc: true},
	{Name: "dart", Class: LangCLike, Exts: []string{".dart"}},
	{Name: "zig", Class: LangCLike, Exts: []string{".zig"}},
	{Name: "groovy", Class: LangCLike, Exts: []string{".groovy", ".gradle"}, Line: "//", Block: slashBlock, Strs: tripleStrs},
	{Name: "solidity", Class: LangCLike, Exts: []string{".sol"}, Line: "//", Block: slashBlock},
	{Name: "move", Class: LangCLike, Exts: []string{".move", ".cairo"}, Line: "//", Block: slashBlock},
	{Name: "odin", Class: LangCLike, Exts: []string{".odin"}, Line: "//", Block: slashBlock, NestedBlock: true},
	{Name: "perl", Class: LangCLike, Exts: []string{".pl", ".pm"}, Line: "#", Strs: tripleStrs, Heredoc: true},
	{Name: "r", Class: LangCLike, Exts: []string{".r"}, Line: "#"},
	{Name: "powershell", Class: LangCLike, Exts: []string{".ps1", ".psm1"}, Line: "#", Block: [2]string{"<#", "#>"}, Heredoc: true},
	{Name: "vue", Class: LangCLike, Exts: []string{".vue", ".svelte", ".astro"}, Line: "//", Block: slashBlock},

	{Name: "ruby", Class: LangOther, Exts: []string{".rb", ".rake", ".gemspec"}, Line: "#", Strs: tripleStrs, Heredoc: true,
		Style: StyleEnd, Func: `^\s*((private|protected|public|module_function)\s+)?def\b`, Container: `^\s*(class|module)\b`,
		Open: `^\s*((private|protected|public|module_function)\s+)?(def|class|module|if|unless|while|until|case|begin|for)\b|[=(]\s*(if|unless|case|begin)\b|\bdo(\s*\|[^|]*\|)?\s*$`},
	{Name: "crystal", Class: LangOther, Exts: []string{".cr"}, Line: "#", Strs: tripleStrs,
		Style: StyleEnd, Func: `^\s*(def|macro)\b`, Container: `^\s*(class|module|struct|lib|enum)\b`,
		Open: `^\s*(def|macro|class|module|struct|lib|enum|if|unless|while|until|case|begin|for)\b|\bdo(\s*\|[^|]*\|)?\s*$`},
	{Name: "elixir", Class: LangOther, Exts: []string{".ex", ".exs"}, Line: "#", Strs: tripleStrs, Heredoc: true,
		Style: StyleEnd, Func: `^\s*(def|defp|defmacro|defmacrop|defguard)\b`, Container: `^\s*(defmodule|defprotocol|defimpl)\b`,
		Open: `^\s*(def|defp|defmacro|defmacrop|defguard|defmodule|defprotocol|defimpl|if|unless|case|cond|with|for|try|receive|quote)\b|\bdo\s*$|\bfn\b.*->\s*$|=\s*(if|case|cond|with|try)\b`},
	{Name: "lua", Class: LangOther, Exts: []string{".lua"}, Line: "--",
		Style: StyleEnd, Func: `\bfunction\b`, Open: `\bfunction\b.*\)\s*$|^\s*(if|for|while)\b.*\b(then|do)\s*$|^\s*do\s*$`},
	{Name: "julia", Class: LangOther, Exts: []string{".jl"}, Line: "#", Strs: tripleStrs,
		Style: StyleEnd, Func: `^\s*(function|macro)\b`, Container: `^\s*(module|struct|mutable struct)\b`,
		Open: `^\s*(function|macro|module|struct|mutable struct|if|for|while|begin|let|try|quote)\b`},
	{Name: "haskell", Class: LangOther, Exts: []string{".hs"}, Line: "--",
		Style: StyleDecl, Func: `^\s*[a-z_][\w']*\b[^:=]*[=|]`, Container: `^\s*(instance|class|data|newtype|module|type|import)\b`},
	{Name: "elm", Class: LangOther, Exts: []string{".elm"}, Line: "--",
		Style: StyleDecl, Func: `^\s*[a-z_][\w']*\b[^:=]*=`, Container: `^\s*(type|module|import)\b`},
	{Name: "ocaml", Class: LangOther, Exts: []string{".ml", ".mli", ".fs", ".fsx"}, Block: [2]string{"(*", "*)"},
		Style: StyleDecl, Func: `^\s*(let|and|member|override|default)\b[^=]*=`, Container: `^\s*(module|type|class|struct|object|namespace|open)\b`},
	{Name: "nim", Class: LangOther, Exts: []string{".nim"}, Line: "#", Strs: tripleStrs,
		Style: StyleDecl, Func: `^\s*(proc|func|method|iterator|template|macro|converter)\b`, Container: `^\s*(type|object)\b`},
	{Name: "clojure", Class: LangOther, Exts: []string{".clj", ".cljs", ".cljc"}, Line: ";",
		Style: StyleParen, Func: `^\((defn-?|def|defmacro|defmethod|defprotocol|defrecord)\b`},
	{Name: "emacs-lisp", Class: LangOther, Exts: []string{".el"}, Line: ";",
		Style: StyleParen, Func: `^\((defun|defmacro|defvar|defconst|defcustom|cl-defun)\b`},
	{Name: "lisp", Class: LangOther, Exts: []string{".lisp", ".lsp"}, Line: ";",
		Style: StyleParen, Func: `^\((defun|defmacro|defvar|defparameter|defgeneric|defmethod|defclass)\b`},
	{Name: "scheme", Class: LangOther, Exts: []string{".scm", ".ss", ".rkt"}, Line: ";",
		Style: StyleParen, Func: `^\((define|define-syntax|define-record-type)\b`},
	{Name: "erlang", Class: LangOther, Exts: []string{".erl", ".hrl"}, Line: "%"},
	{Name: "sql", Class: LangOther, Exts: []string{".sql"}, Line: "--", Block: slashBlock},
	{Name: "protobuf", Class: LangOther, Exts: []string{".proto"}, Line: "//", Block: slashBlock},
	{Name: "graphql", Class: LangOther, Exts: []string{".graphql", ".gql"}, Line: "#"},
	{Name: "terraform", Class: LangOther, Exts: []string{".tf", ".hcl"}, Line: "#", Strs: tripleStrs, Heredoc: true},

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
	name, class string // a language row by name, else just the class
	re          *lazyre.Regexp
}{
	{"", LangGo, lazyre.New(`(?m)^package \w+\s*$`)},
	{"", LangPython, lazyre.New(`(?m)^(def \w+\(.*\):|class \w+(\(.*\))?:|from [\w.]+ import |import \w+\s*$)`)},
	{"", LangShell, lazyre.New(`^#!.*\b(ba|z)?sh\b`)},
	{"ruby", LangOther, lazyre.New(`^#!.*\bruby\b`)},
	{"perl", LangCLike, lazyre.New(`^#!.*\bperl\b`)},
	{"php", LangCLike, lazyre.New(`^#!.*\bphp\b`)},
	{"lua", LangOther, lazyre.New(`^#!.*\blua(jit)?\b`)},
	{"javascript", LangCLike, lazyre.New(`^#!.*\b(node|deno|bun)\b`)},
	{"d", LangCLike, lazyre.New(`^#!.*\brdmd\b`)},
	{"", LangPython, lazyre.New(`^#!.*\bpython`)},
	// Before the generic c-like sniffer, which would take Rust for it.
	{"rust", LangCLike, lazyre.New(`(?m)^\s*(pub(\([a-z]+\))? )?(async |const |unsafe )*fn \w+[<(]|^\s*impl(<[^>]*>)? [\w:<>, ]+( for \w+)? \{|^\s*#\[(derive|cfg|test)\b|^\s*(pub )?mod \w+[;{]`)},
	{"", LangCLike, lazyre.New(`(?m)^(import .* from |export (default |const |function |class |interface )|#include [<"]|(public |private )?(static )?(class|interface|enum) \w+.*\{|fn \w+\(|use [\w:]+;|function \w+\()`)},
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

// Source is the language of source code: its row by file extension, else a
// guess from the text (a row only for languages told apart by content).
// ok is false when text is not code.
func Source(text string, h Hints) (l Language, ok bool) {
	if l, ok := LanguageOf(h); ok {
		return l, true
	}
	head := text[:min(len(text), 8192)]
	for _, s := range langSniffers {
		if !s.re.MatchString(head) {
			continue
		}
		if i := slices.IndexFunc(languages, func(l Language) bool { return l.Name == s.name }); i >= 0 {
			return languages[i], true
		}
		return Language{Class: s.class}, true
	}
	return Language{}, false
}

// Lang returns the language class of source code, or "" when text is not code.
func Lang(text string, h Hints) string {
	l, _ := Source(text, h)
	return l.Class
}
