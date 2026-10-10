// Package docoptimizer is the doc family engine: markdown and reStructuredText
// lose badges, comments, layout HTML and tables of contents and keep long code
// blocks at their ends, long markdown keeps the sections that match the query; man pages and --help output lose page furniture,
// boilerplate sections and deep indentation.
package docoptimizer

import (
	"context"
	"regexp"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyDoc {
			continue
		}
		var out []string
		switch detect.Dialect(b.Type) {
		case "markdown":
			out = o.markdown(compress.Lines(htmlComment.ReplaceAllString(b.Text, "")), p.Attrs[payload.AttrQuery])
		case "rst":
			out = o.rst(compress.Lines(b.Text))
		case "man":
			out = o.man(compress.Lines(compress.StripANSI(overstrike.ReplaceAllString(b.Text, ""))))
		case "help":
			out = help(compress.Lines(compress.StripANSI(b.Text)))
		default:
			continue
		}
		p.Blocks[i].Text = compress.Join(compress.SqueezeBlank(out))
	}
	return nil
}

var (
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	fence       = regexp.MustCompile("^\\s*(```|~~~)")
	// Badges: linked images, shields and CI status pictures.
	badge = regexp.MustCompile(`\[!\[[^\]]*\]\([^)]*\)\]\([^)]*\)|!\[[^\]]*\]\([^)]*(shields\.io|badge|/workflows/|codecov|travis-ci|goreportcard|pkg\.go\.dev/badge)[^)]*\)`)
	image = regexp.MustCompile(`!\[([^\]]*)\]\([^)]*\)`)
	// Lines of layout HTML only: <p align="center">, <img …>, </div>, <br/>.
	layoutHTML = regexp.MustCompile(`^\s*(</?(p|div|span|center|picture|source|br|hr|a|img|h\d|sup|sub|details|summary)\b[^>]*>\s*)+$`)
	tocItem    = regexp.MustCompile(`^\s*([-*+]|\d+\.) \[[^\]]+\]\(#[^)]*\)\s*$`)
)

// markdown drops badges, images, layout HTML and tables of contents, keeps
// long code blocks at their ends and fits long pages into max_tokens.
func (o *Optimizer) markdown(lines []string, query string) []string {
	var out, code []string
	inCode, toc := false, 0
	for _, l := range lines {
		if fence.MatchString(l) {
			if inCode {
				out = append(out, o.code(code)...)
				code = nil
			}
			inCode = !inCode
			out = append(out, l)
			continue
		}
		if inCode {
			code = append(code, l)
			continue
		}
		if tocItem.MatchString(l) {
			toc++
			continue
		}
		l = badge.ReplaceAllString(l, "")
		l = image.ReplaceAllStringFunc(l, func(m string) string {
			if alt := image.FindStringSubmatch(m)[1]; alt != "" {
				return "[image: " + alt + "]"
			}
			return ""
		})
		if layoutHTML.MatchString(l) || strings.TrimSpace(l) == "" && l != "" {
			continue
		}
		out = append(out, l)
	}
	out = append(out, o.code(code)...) // unclosed fence
	out = sections(out, query, o.cfg.MaxTokens)
	if toc > 0 {
		out = append(out, compress.Omitted(toc, "table of contents entries"))
	}
	return out
}

// code keeps the head and tail of a long code block.
func (o *Optimizer) code(lines []string) []string {
	n := o.cfg.MaxCodeLines
	return compress.HeadTail(lines, n*2/3, n/3)
}

var (
	rstDirective = regexp.MustCompile(`^\.\. (\|[^|]+\| )?(image|figure|raw|contents|toctree|only|meta|highlight|index)::`)
	rstComment   = regexp.MustCompile(`^\.\.(\s|$)`)
	rstCodeStart = regexp.MustCompile(`(::\s*$|^\.\. (code-block|code|sourcecode)::)`)
)

// rst drops images, badges, tables of contents and comments with their
// indented bodies, and keeps long literal blocks at their ends.
func (o *Optimizer) rst(lines []string) []string {
	var out, code []string
	skip, inCode := false, false
	for _, l := range lines {
		indented := strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")
		if (skip || inCode) && (indented || strings.TrimSpace(l) == "") {
			if inCode {
				code = append(code, l)
			}
			continue
		}
		if inCode {
			out = append(out, o.code(code)...)
			code, inCode = nil, false
		}
		skip = rstDirective.MatchString(l) || rstComment.MatchString(l) && !strings.Contains(l, "::")
		if skip {
			continue
		}
		out = append(out, l)
		inCode = rstCodeStart.MatchString(l)
	}
	return append(out, o.code(code)...)
}

var (
	overstrike = regexp.MustCompile(`.\x08`) // bold/underline in raw man output
	manHeader  = regexp.MustCompile(`^\S+\(\d\w*\)\s.*\S+\(\d\w*\)$`)
	section    = regexp.MustCompile(`^[A-Z][A-Z0-9 ]+$`)
	justified  = regexp.MustCompile(`(\S)  +`) // spaces groff adds to justify lines
	footer     = regexp.MustCompile(`^\S.*\s{3,}\d{4}-\d\d-\d\d\s{3,}\S+\(\d\w*\)$|^\S.*\s{3,}\S.*\s{3,}\S+\(\d\w*\)$`)
)

// man drops page headers and footers and boilerplate sections, undoes
// justification and dedents the body.
func (o *Optimizer) man(lines []string) []string {
	var out []string
	dropping := false
	for _, l := range lines {
		if manHeader.MatchString(l) || footer.MatchString(l) {
			continue
		}
		if section.MatchString(l) {
			dropping = slices.Contains(o.cfg.ManDrop, l)
			if !dropping {
				out = append(out, l)
			}
			continue
		}
		if !dropping {
			out = append(out, justified.ReplaceAllString(l, "$1 "))
		}
	}
	return dedent(out)
}

var helpBoilerplate = regexp.MustCompile(`^(Report (any translation )?bugs to|.*Full documentation <|or available locally via: info|.*online help: <|General help using GNU software|GNU \w+ home page: <)`)

// help drops GNU boilerplate and dedents.
func help(lines []string) []string {
	var out []string
	for _, l := range lines {
		if !helpBoilerplate.MatchString(l) {
			out = append(out, l)
		}
	}
	return dedent(out)
}

// dedent halves indentation of 4+ spaces: man pages indent body by 7 and
// option descriptions by 14.
func dedent(lines []string) []string {
	for i, l := range lines {
		trimmed := strings.TrimLeft(l, " ")
		if n := len(l) - len(trimmed); n >= 4 {
			lines[i] = strings.Repeat(" ", (n+1)/2) + trimmed
		}
	}
	return lines
}
