// Package docoptimizer is the doc family engine: markdown and reStructuredText
// lose badges, comments, layout HTML and tables of contents and keep long code
// blocks at their ends, long markdown keeps the sections that match the query; man pages and --help output lose page furniture,
// boilerplate sections and deep indentation.
package docoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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
			out = o.man(compress.CleanLines(overstrike.ReplaceAllString(b.Text, "")))
		case "help":
			out = helpFor(help(compress.CleanLines(b.Text)), p.Attrs[payload.AttrQuery])
		default:
			continue
		}
		p.Blocks[i].Text = compress.Join(compress.SqueezeBlank(out))
	}
	return nil
}

var (
	htmlComment = lazyre.New(`(?s)<!--.*?-->`)
	// Badges: linked images, shields and CI status pictures.
	badge = lazyre.New(`\[!\[[^\]]*\]\([^)]*\)\]\([^)]*\)|!\[[^\]]*\]\([^)]*(shields\.io|badge|/workflows/|codecov|travis-ci|goreportcard|pkg\.go\.dev/badge)[^)]*\)`)
	image = lazyre.New(`!\[([^\]]*)\]\([^)]*\)`)
	// Lines of layout HTML only: <p align="center">, <img …>, </div>, <br/>.
	layoutHTML = lazyre.New(`^\s*(</?(p|div|span|center|picture|source|br|hr|a|img|h\d|sup|sub|details|summary)\b[^>]*>\s*)+$`)
	tocItem    = lazyre.New(`^\s*([-*+]|\d+\.) \[[^\]]+\]\(#[^)]*\)\s*$`)
)

// markdown drops badges, images, layout HTML and tables of contents, keeps
// long code blocks at their ends and fits long pages into max_tokens.
func (o *Optimizer) markdown(lines []string, query string) []string {
	var out, code []string
	inCode, toc := false, 0
	for _, l := range lines {
		if compress.IsFence(l) {
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
		if strings.Contains(l, "![") { // both patterns need it; skips the regexes on most lines
			l = badge.ReplaceAllString(l, "")
			l = image.ReplaceAllStringFunc(l, func(m string) string {
				if alt := image.FindStringSubmatch(m)[1]; alt != "" {
					return "[image: " + alt + "]"
				}
				return ""
			})
		}
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
	rstDirective = lazyre.New(`^\.\. (\|[^|]+\| )?(image|figure|raw|contents|toctree|only|meta|highlight|index)::`)
	rstComment   = lazyre.New(`^\.\.(\s|$)`)
	rstTarget    = lazyre.New(`^\.\. _[^:]+: \S`) // link target: the URL a `name`_ reference points to
	rstImageSub  = lazyre.New(`^\.\. \|([^|]+)\| (image|figure)::`)
	rstSubRef    = lazyre.New(`\|([^|\s][^|]*)\|`)
	rstCodeStart = lazyre.New(`(::\s*$|^\.\. (code-block|code|sourcecode)::)`)
)

// rst drops images, badges, tables of contents and comments with their
// indented bodies, and keeps long literal blocks at their ends.
func (o *Optimizer) rst(lines []string) []string {
	lines = dropBadgeRows(lines)
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
		skip = rstDirective.MatchString(l) || rstComment.MatchString(l) && !strings.Contains(l, "::") && !rstTarget.MatchString(l)
		if skip {
			continue
		}
		out = append(out, l)
		inCode = rstCodeStart.MatchString(l)
	}
	return append(out, o.code(code)...)
}

var (
	overstrike = lazyre.New(`.\x08`) // bold/underline in raw man output
	manHeader  = lazyre.New(`^\S+\(\d\w*\)\s.*\S+\(\d\w*\)$`)
	section    = lazyre.New(`^[A-Z][A-Z0-9 ]+$`)
	justified  = lazyre.New(`(\S)  +`) // spaces groff adds to justify lines
	footer     = lazyre.New(`^\S.*\s{3,}\d{4}-\d\d-\d\d\s{3,}\S+\(\d\w*\)$|^\S.*\s{3,}\S.*\s{3,}\S+\(\d\w*\)$`)
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

var helpBoilerplate = lazyre.New(`^(Report (any translation )?bugs to|.*Full documentation <|or available locally via: info|.*online help: <|General help using GNU software|GNU \w+ home page: <)`)

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

// optionLine starts an option entry: "  -a, --append   text", "  --basic".
var optionLine = lazyre.New(`^\s{1,8}-{1,2}\w`)

// minOptions is the fewest option entries worth filtering by a query.
const minOptions = 20

// helpFor keeps, with a query, only the option entries matching it (at
// least a third of the best match), plus everything that is not an option
// (usage, section headings, prose): every line of help text is distinct, so
// without a query there is nothing to drop.
func helpFor(lines []string, query string) []string {
	terms := sample.Terms(query)
	if len(terms) == 0 {
		return lines
	}
	// Option entries: an option line and its indented continuation lines.
	var starts, ends []int
	for i := 0; i < len(lines); i++ {
		if !optionLine.MatchString(lines[i]) {
			continue
		}
		j := i + 1
		for j < len(lines) && strings.HasPrefix(lines[j], " ") && !optionLine.MatchString(lines[j]) && strings.TrimSpace(lines[j]) != "" {
			j++
		}
		starts, ends = append(starts, i), append(ends, j)
		i = j - 1
	}
	if len(starts) < minOptions {
		return lines
	}
	docs := make([][]string, len(starts))
	for k := range starts {
		docs[k] = sample.Terms(strings.Join(lines[starts[k]:ends[k]], " "))
	}
	scores := sample.BM25(docs, terms)
	best := slices.Max(scores)
	if best == 0 {
		return lines
	}
	var out []string
	dropped, k := 0, 0
	for i := 0; i < len(lines); {
		if k < len(starts) && i == starts[k] {
			if scores[k] >= best/3 {
				if dropped > 0 {
					out = append(out, compress.Omitted(dropped, "options"))
					dropped = 0
				}
				out = append(out, lines[starts[k]:ends[k]]...)
			} else {
				dropped++
			}
			i = ends[k]
			k++
			continue
		}
		if dropped > 0 {
			out = append(out, compress.Omitted(dropped, "options"))
			dropped = 0
		}
		out = append(out, lines[i])
		i++
	}
	if dropped > 0 {
		out = append(out, compress.Omitted(dropped, "options"))
	}
	return out
}

// dropBadgeRows removes lines made only of image substitutions
// ("| |pypi| |license|"): their definitions, the badge images, go too.
func dropBadgeRows(lines []string) []string {
	images := map[string]bool{}
	for _, l := range lines {
		if m := rstImageSub.FindStringSubmatch(l); m != nil {
			images[m[1]] = true
		}
	}
	if len(images) == 0 {
		return lines
	}
	var out []string
	for _, l := range lines {
		refs := rstSubRef.FindAllStringSubmatch(l, -1)
		rest := strings.Trim(rstSubRef.ReplaceAllString(l, ""), "| \t")
		if len(refs) > 0 && rest == "" && !slices.ContainsFunc(refs, func(m []string) bool { return !images[m[1]] }) {
			continue
		}
		out = append(out, l)
	}
	return out
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
