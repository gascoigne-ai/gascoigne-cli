package symbols

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	// endName is the name a def/class/module/function line gives, "self."
	// dropped; Lua's M.run and M:run and Elixir's Mod.Name stay whole.
	endName = lazyre.New(`\b(?:defp|defmacrop|defmacro|defguard|defmodule|defprotocol|defimpl|def|macro|function|class|module|struct|enum|lib)\s+(?:self\.)?([\w.:?!]+)`)
	// endOneLine is a block opened and closed on its line: def f; end,
	// def f(x), do: x, or an endless def f(x) = x.
	endOneLine = lazyre.New(`\bend\b[\s);,]*$|(^|[\s,])do:\s|\bdef\s[^=]*\)\s*=[^=~>]`)
)

// endDefs finds definitions in languages that close blocks with end (ruby,
// elixir, lua…), using the block patterns of the language row.
func endDefs(lines []string, row detect.Language) []Def {
	open, fn, cont := lazyre.New(row.Open), lazyre.New(row.Func), lazyre.New(row.Container)
	type block struct {
		def int // index in defs, -1 for other blocks
	}
	var defs []Def
	var stack []block
	for i, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "" || row.Line != "" && strings.HasPrefix(t, row.Line):
		case strings.HasPrefix(t, "end") && (len(t) == 3 || !isWord(t[3])):
			if len(stack) == 0 {
				continue
			}
			if top := stack[len(stack)-1]; top.def >= 0 {
				defs[top.def].To = i + 1
			}
			stack = stack[:len(stack)-1]
		case open.MatchString(l):
			def := -1
			kind := ""
			switch {
			case row.Container != "" && cont.MatchString(l):
				kind = "type"
			case row.Func != "" && fn.MatchString(l):
				kind = "func"
			}
			if m := endName.FindStringSubmatch(l); m != nil && kind != "" {
				name := m[1]
				for k := len(stack) - 1; k >= 0; k-- {
					if stack[k].def >= 0 {
						name = defs[stack[k].def].Name + "." + name
						if kind == "func" {
							kind = "method"
						}
						break
					}
				}
				def = len(defs)
				defs = append(defs, Def{Name: name, Kind: kind, From: docStart(lines, i, row) + 1, Line: i + 1, To: i + 1})
			}
			if !endOneLine.MatchString(t) {
				stack = append(stack, block{def})
			}
		}
	}
	// A def whose end never came (a pattern this table misses) is dropped
	// rather than shown with a wrong range.
	return slices.DeleteFunc(defs, func(d Def) bool {
		return d.To == d.Line && !endOneLine.MatchString(strings.TrimSpace(lines[d.Line-1]))
	})
}

// docStart is the first line of the comments (and @doc/@spec attributes)
// right above line i.
func docStart(lines []string, i int, row detect.Language) int {
	from := i
	for from > 0 {
		t := strings.TrimSpace(lines[from-1])
		switch {
		case row.Line != "" && strings.HasPrefix(t, row.Line), strings.HasPrefix(t, "@") && len(t) > 1:
			from--
		case t == `"""`: // end of an @doc heredoc: back to its opening line
			j := from - 2
			for j >= 0 && !strings.Contains(lines[j], `"""`) {
				j--
			}
			if j < 0 || !strings.HasPrefix(strings.TrimSpace(lines[j]), "@") {
				return from
			}
			from = j
		default:
			return from
		}
	}
	return from
}

func isWord(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// mdHead is an ATX heading: 1-6 hashes, a space, the title.
var mdHead = lazyre.New("^(#{1,6})\\s+(.+?)(?:\\s+#+)?\\s*$")

// mdDefs lists the headings of a Markdown file as sections: a section runs to
// the line before the next heading of the same or a higher level.
func mdDefs(lines []string) []Def {
	var defs []Def
	var levels []int
	fence := ""
	for i, l := range lines {
		if f := fenceOf(l); f != "" {
			switch {
			case fence == "":
				fence = f
			case f == fence:
				fence = ""
			}
			continue
		}
		m := mdHead.FindStringSubmatch(l)
		if fence != "" || m == nil {
			continue
		}
		for k := len(defs) - 1; k >= 0 && levels[k] >= len(m[1]); k-- {
			if defs[k].To == 0 {
				defs[k].To = i
			}
		}
		defs = append(defs, Def{Name: m[2], Kind: "section", From: i + 1, Line: i + 1})
		levels = append(levels, len(m[1]))
	}
	for k := range defs {
		if defs[k].To == 0 {
			defs[k].To = len(lines)
		}
		for defs[k].To > defs[k].Line && strings.TrimSpace(lines[defs[k].To-1]) == "" {
			defs[k].To--
		}
	}
	return defs
}

// fenceOf is the ``` or ~~~ opening or closing a fenced code block, else "".
func fenceOf(l string) string {
	t := strings.TrimLeft(l, " ")
	for _, f := range []string{"```", "~~~"} {
		if strings.HasPrefix(t, f) {
			return f
		}
	}
	return ""
}
