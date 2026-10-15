// Package catoptimizer shrinks source files read by agents. In auto mode
// small files pass untouched, medium ones are cleaned (comment runs dropped,
// the first line of a doc comment or docstring kept) and large ones also
// become outlines (function bodies elided: Go via go/ast, brace and indent
// heuristics for other languages). Lines are only dropped whole, never
// rewritten, and every gap is one marker naming the original lines
// ("// … comments L12-30"), so what an agent sees can go into an edit as is
// and the rest can be read back by line range. Mode can be forced per run
// with attr cat_mode or env GASCOIGNE_CAT_MODE.
package catoptimizer

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	mode := cmp.Or(p.Attrs[AttrMode], os.Getenv(EnvMode), o.cfg.Mode)
	if !validMode(mode) {
		return fmt.Errorf("invalid %s %q", AttrMode, mode)
	}
	hints := detect.HintsFrom(p.Attrs)
	return p.Rewrite(func(b payload.Block) (string, error) {
		text := o.rewrite(b.Text, detect.Lang(b.Text, hints), hints, mode)
		if text == b.Text {
			p.Set(payload.AttrSkip, o.why(b.Text, mode))
		}
		return text, nil
	}, detect.Code)
}

// why explains a source file left untouched.
func (o *Optimizer) why(text, mode string) string {
	n := tokens.Estimate(text)
	switch {
	case mode == Full:
		return "cat: " + AttrMode + "=full"
	case mode == Auto && n < o.cfg.CleanMinTokens:
		return fmt.Sprintf("cat: %d tokens < clean_min_tokens %d (small files stay whole)", n, o.cfg.CleanMinTokens)
	}
	return "cat: already clean, nothing to outline"
}

func (o *Optimizer) rewrite(text, lang string, h detect.Hints, mode string) string {
	if mode == Auto {
		switch n := tokens.Estimate(text); {
		case n < o.cfg.CleanMinTokens:
			mode = Full
		case n < o.cfg.OutlineMinTokens:
			mode = Clean
		default:
			mode = Outline
		}
	}
	if mode == Full {
		return text
	}
	lines := compress.Lines(text)
	row, _ := detect.Source(text, h)
	if row.Class != lang {
		row = detect.Language{}
	}
	var bodies []span
	if mode == Outline {
		bodies = o.outline(text, lines, lang, row)
	}
	gaps := bodies
	syn, known := syntaxOf(lang, h)
	if known {
		gaps = merge(bodies, comments(lines, syn))
	} else {
		syn.line = comment(lang)
	}
	if len(gaps) == 0 {
		return text
	}
	// tail shows the end of a file: its line numbers are not the file's.
	numbered := !slices.Contains([]string{"tail", "sed", "awk"}, firstArg(h.Cmd))
	out := render(lines, gaps, cmp.Or(syn.line, syn.block[0]), numbered)
	if len(bodies) > 0 {
		note := fmt.Sprintf("%s outline: %d bodies elided (%s=full for complete source)", cmp.Or(syn.line, syn.block[0]), len(bodies), AttrMode)
		out = append([]string{note}, out...)
	}
	return compress.Join(out)
}

func (o *Optimizer) outline(text string, lines []string, lang string, row detect.Language) []span {
	switch lang {
	case detect.LangGo:
		return o.goOutline(text, lines)
	case detect.LangPython:
		return o.indentOutline(lines)
	case detect.LangCLike:
		return o.braceOutline(lines, row)
	}
	return nil
}

func comment(lang string) string {
	if lang == detect.LangPython || lang == detect.LangShell || lang == detect.LangOther {
		return "#"
	}
	return "//"
}

func firstArg(cmd string) string {
	if args := detect.Argv(cmd); len(args) > 0 {
		return args[0]
	}
	return ""
}

// merge adds comment spans outside elided bodies, in line order.
func merge(bodies, comments []span) []span {
	gaps := slices.Clone(bodies)
	for _, c := range comments {
		inside := slices.ContainsFunc(bodies, func(b span) bool { return c.from <= b.to && b.from <= c.to })
		if !inside {
			gaps = append(gaps, c)
		}
	}
	slices.SortFunc(gaps, func(a, b span) int { return a.from - b.from })
	return gaps
}

// render keeps lines outside gaps byte-exact and puts one marker per gap,
// naming the original lines so an agent can read exactly those back:
// "// … comments L12-30", "… L50-89" for an elided body.
func render(lines []string, gaps []span, token string, numbered bool) []string {
	var out []string
	next := 0
	for _, g := range gaps {
		out = append(out, lines[next:g.from]...)
		indent := g.indent
		if g.what == "" { // elided body: indent of its first line
			indent = lines[g.from][:indentOf(lines[g.from])]
		}
		where := fmt.Sprintf("%d lines", g.to-g.from+1)
		if numbered {
			where = lineRange(g.from+1, g.to+1)
		}
		marker := strings.Join(slices.DeleteFunc([]string{indent + token, "…", g.what, where}, func(s string) bool { return s == "" }), " ")
		out = append(out, marker)
		next = g.to + 1
	}
	return append(out, lines[next:]...)
}

func lineRange(from, to int) string {
	if from == to {
		return fmt.Sprintf("L%d", from)
	}
	return fmt.Sprintf("L%d-%d", from, to)
}
