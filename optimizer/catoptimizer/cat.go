// Package catoptimizer shrinks source files read by agents. In auto mode
// small files pass untouched, medium ones lose license headers and blank
// runs, and large ones become outlines: declarations and signatures kept,
// function bodies elided (Go via go/ast, brace and indent heuristics for
// other languages). Mode can be forced per run with attr cat_mode or env
// GASCOIGNE_CAT_MODE.
package catoptimizer

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/tokens"
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
		return o.rewrite(b.Text, detect.Lang(b.Text, hints), mode), nil
	}, detect.Code)
}

func (o *Optimizer) rewrite(text, lang, mode string) string {
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
	lines := clean(compress.Lines(text))
	if mode == Clean {
		return compress.Join(lines)
	}
	var out []string
	var elided int
	switch lang {
	case detect.LangGo:
		out, elided = o.goOutline(compress.Join(lines))
	case detect.LangPython:
		out, elided = o.indentOutline(lines)
	case detect.LangCLike:
		out, elided = o.braceOutline(lines)
	default:
		return compress.Join(lines)
	}
	if elided == 0 {
		return compress.Join(lines)
	}
	if lang != detect.LangPython {
		out = collapseComments(out)
	}
	note := fmt.Sprintf("%s outline: %d bodies elided (%s=full for complete source)", comment(lang), elided, AttrMode)
	return compress.Join(append([]string{note}, out...))
}

func comment(lang string) string {
	if lang == detect.LangPython || lang == detect.LangShell {
		return "#"
	}
	return "//"
}

var license = regexp.MustCompile(`(?i)licen[cs]e|copyright|\(c\) \d{4}`)

// clean drops a leading license comment, trailing spaces and blank runs.
func clean(lines []string) []string {
	end := 0
	for end < len(lines) && isComment(lines[end]) {
		end++
	}
	if end > 0 && license.MatchString(strings.Join(lines[:end], "\n")) {
		lines = lines[end:]
		for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
			lines = lines[1:]
		}
	}
	return compress.SqueezeBlank(lines)
}

func isComment(l string) bool {
	t := strings.TrimSpace(l)
	return strings.HasPrefix(t, "//") || strings.HasPrefix(t, "/*") || strings.HasPrefix(t, "*") ||
		strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!") && !strings.HasPrefix(t, "#include")
}

// collapseComments keeps the first line of each run of 3+ line comments:
// in an outline the summary sentence is what matters.
func collapseComments(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i
		for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "//") {
			j++
		}
		if j-i >= 3 {
			indent := lines[i][:len(lines[i])-len(strings.TrimLeft(lines[i], " \t"))]
			out = append(out, lines[i], fmt.Sprintf("%s// … %d comment lines", indent, j-i-1))
			i = j
			continue
		}
		if j == i {
			j++
		}
		out = append(out, lines[i:j]...)
		i = j
	}
	return out
}

func elision(indent string, n int) string {
	return fmt.Sprintf("%s… %d lines", indent, n)
}
