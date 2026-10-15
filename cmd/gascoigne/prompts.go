package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/terse"
	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

// Instruction files are expanded the way the agents document it: imports
// up to four hops deep, MEMORY.md cut at 200 lines or 25 KB, block HTML
// comments stripped (they never reach the model).
const (
	maxImportDepth = 4
	memoryLines    = 200
	memoryBytes    = 25 << 10
)

var (
	importRe  = lazyre.New(`(?:^|\s)@((?:~/|\.{0,2}/)?[\w.\-/]+\.\w+)`)
	commentRe = lazyre.New(`(?s)<!--.*?-->`)
	codeRe    = lazyre.New("`[^`\n]*`")
)

// promptSource is one instruction text an agent sends with every request.
type promptSource struct {
	Path   string `json:"path"`
	Via    string `json:"via"`    // project, user, memory, @import from <path>, skill, agent
	Tokens int    `json:"tokens"` // the model's own tokens
	text   string
	full   bool // whole file sent, not only its description
}

// promptFinding is a fact about instruction text worth changing.
type promptFinding struct {
	Kind   string `json:"kind"` // large-import, duplicate, long-description
	Path   string `json:"path"`
	Tokens int    `json:"tokens"` // model tokens it costs per request
	Detail string `json:"detail"`
}

// cmdPrompts lists the instruction text an agent sends with every request
// and what it costs.
func cmdPrompts(_ context.Context, args []string) error {
	fs := newFlags("prompts")
	agent := fs.String("agent", "claude", "agent whose instruction files to read: claude or codex")
	model := fs.String("model", "", "model to price (default: the agent's)")
	large := fs.Int("large", 2000, "imports of at least `n` model tokens are reported")
	asJSON := fs.Bool("json", false, "print JSON instead of a table")
	terse := fs.Float64("terse", 0, "instead of listing: drop up to this `share` (0-1) of the words of the files given, articles and fillers first; each original kept as <file>.bak")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *terse > 0 {
		return terseFiles(fs.Args(), *terse)
	}
	dir, err := filepath.Abs(cmp.Or(fs.Arg(0), "."))
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	m, ok := cost.Lookup(cmp.Or(*model, cost.AgentModels[*agent]))
	if !ok {
		return fmt.Errorf("unknown model %q", *model)
	}
	var srcs []promptSource
	switch *agent {
	case "claude":
		srcs = claudeSources(dir, home)
	case "codex":
		srcs = codexSources(dir, home)
	default:
		return fmt.Errorf("unknown agent %q: claude or codex", *agent)
	}
	factor := cost.Tokenizers[m.Tokenizer].Factor
	total := 0
	for i := range srcs {
		srcs[i].Tokens = int(float64(tokens.Estimate(srcs[i].text))*factor + 0.5)
		total += srcs[i].Tokens
		srcs[i].Path = shortPath(srcs[i].Path, dir, home)
		if from, ok := strings.CutPrefix(srcs[i].Via, "@import from "); ok {
			srcs[i].Via = "@import from " + shortPath(from, dir, home)
		}
	}
	findings := promptFindings(srcs, *large, factor)
	s := cost.Estimate(m, int(float64(total)/factor+0.5), cost.AgentContext)
	perRequest := float64(s.ModelTokens) * m.CacheRead / 1e6
	if *asJSON {
		return writeJSON(map[string]any{"agent": *agent, "model": m.ID, "tokens": s.ModelTokens,
			"per_request_usd": perRequest, "per_session_usd": s.InContext, "sources": srcs, "findings": findings})
	}
	printPrompts(*agent, m.ID, s, perRequest, srcs, findings)
	return nil
}

// claudeSources are Claude Code's instruction files for dir: the user's
// CLAUDE.md, every CLAUDE.md from the root down to dir with its imports,
// auto memory, and the descriptions of skills and subagents it lists.
func claudeSources(dir, home string) []promptSource {
	l := &sourceLoader{seen: map[string]bool{}}
	if home != "" {
		l.load(filepath.Join(home, ".claude", "CLAUDE.md"), "user", 0)
	}
	for _, d := range ancestors(dir) {
		for _, name := range []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md"), "CLAUDE.local.md"} {
			l.load(filepath.Join(d, name), "project", 0)
		}
	}
	if home != "" {
		l.memory(filepath.Join(home, ".claude", "projects", projectKey(dir), "memory", "MEMORY.md"))
	}
	for _, base := range []string{filepath.Join(dir, ".claude"), filepath.Join(home, ".claude")} {
		l.descriptions(filepath.Join(base, "skills", "*", "SKILL.md"), "skill")
		l.descriptions(filepath.Join(base, "agents", "*.md"), "agent")
	}
	return l.srcs
}

// codexSources are the user's AGENTS.md and each one from the repository
// root down to dir.
func codexSources(dir, home string) []promptSource {
	l := &sourceLoader{seen: map[string]bool{}}
	if home != "" {
		l.load(filepath.Join(home, ".codex", "AGENTS.md"), "user", maxImportDepth)
	}
	root := ancestors(dir)
	for i := len(root) - 1; i >= 0; i-- {
		if _, err := os.Stat(filepath.Join(root[i], ".git")); err == nil {
			root = root[i:]
			break
		}
	}
	for _, d := range root {
		l.load(filepath.Join(d, "AGENTS.md"), "project", maxImportDepth)
	}
	return l.srcs
}

type sourceLoader struct {
	srcs []promptSource
	seen map[string]bool
}

// load adds a file and, below maxImportDepth, the files it imports.
func (l *sourceLoader) load(path, via string, depth int) {
	if l.seen[path] {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	l.seen[path] = true
	text := commentRe.ReplaceAllString(string(data), "")
	l.srcs = append(l.srcs, promptSource{Path: path, Via: via, text: text, full: true})
	if depth >= maxImportDepth {
		return
	}
	for _, imp := range imports(text) {
		target := imp
		if rest, ok := strings.CutPrefix(imp, "~/"); ok {
			home, _ := os.UserHomeDir()
			target = filepath.Join(home, rest)
		} else if !filepath.IsAbs(imp) {
			target = filepath.Join(filepath.Dir(path), imp)
		}
		l.load(target, "@import from "+path, depth+1)
	}
}

// memory adds the part of auto memory's index the agent loads.
func (l *sourceLoader) memory(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	text := string(data[:min(len(data), memoryBytes)])
	if lines := strings.SplitAfter(text, "\n"); len(lines) > memoryLines {
		text = strings.Join(lines[:memoryLines], "")
	}
	l.srcs = append(l.srcs, promptSource{Path: path, Via: "memory", text: text, full: true})
}

// descriptions adds the name and description of each file matching glob:
// what a skill or subagent listing sends until one is used.
func (l *sourceLoader) descriptions(glob, via string) {
	paths, _ := filepath.Glob(glob)
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil || l.seen[p] {
			continue
		}
		l.seen[p] = true
		if text := frontmatter(string(data)); text != "" {
			l.srcs = append(l.srcs, promptSource{Path: p, Via: via, text: text})
		}
	}
}

// imports lists the @paths of a markdown text, outside code fences and
// code spans (the agent skips those too).
func imports(text string) []string {
	var out []string
	fenced := false
	for line := range strings.Lines(text) {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		for _, m := range importRe.FindAllStringSubmatch(codeRe.ReplaceAllString(line, ""), -1) {
			out = append(out, m[1])
		}
	}
	return out
}

// frontmatter returns the name and description lines of a file's YAML
// frontmatter.
func frontmatter(text string) string {
	body, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	head, _, _ := strings.Cut(body, "\n---")
	var keep []string
	for line := range strings.Lines(head) {
		if strings.HasPrefix(line, "name:") || strings.HasPrefix(line, "description:") {
			keep = append(keep, strings.TrimSpace(line))
		}
	}
	return strings.Join(keep, "\n")
}

// ancestors lists dir and its parents, root first.
func ancestors(dir string) []string {
	var out []string
	for d := dir; ; d = filepath.Dir(d) {
		out = append(out, d)
		if filepath.Dir(d) == d {
			break
		}
	}
	slices.Reverse(out)
	return out
}

// projectKey names a project directory the way Claude Code does under
// ~/.claude/projects: every character other than a letter or digit is a dash.
func projectKey(dir string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, dir)
}

// shortPath shows a path relative to dir when inside it, else with ~.
func shortPath(p, dir, home string) string {
	if rel, err := filepath.Rel(dir, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	if home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}

// Lines shorter than this are too common to call duplicates; skill
// descriptions longer than this weigh on every request's listing.
const (
	dupMinChars   = 40
	descMaxTokens = 120
)

// promptFindings reports large imports, lines sent twice and long
// descriptions.
func promptFindings(srcs []promptSource, large int, factor float64) []promptFinding {
	var out []promptFinding
	model := func(s string) int { return int(float64(tokens.Estimate(s))*factor + 0.5) }
	first := map[string]string{} // normalized line → path that sent it first
	type pair struct{ path, earlier string }
	dups := map[pair][]string{}
	var order []pair
	for _, s := range srcs {
		if !s.full {
			if s.Tokens > descMaxTokens {
				out = append(out, promptFinding{"long-description", s.Path, s.Tokens,
					"listed with every request; say when to use it, the body loads on use"})
			}
			continue
		}
		if strings.HasPrefix(s.Via, "@import") && s.Tokens >= large {
			out = append(out, promptFinding{"large-import", s.Path, s.Tokens,
				"sent with every request; make it a skill or a file the agent reads when needed"})
		}
		for line := range strings.Lines(s.text) {
			key := strings.Join(strings.Fields(strings.ToLower(line)), " ")
			if len(key) < dupMinChars {
				continue
			}
			earlier, ok := first[key]
			if !ok {
				first[key] = s.Path
				continue
			}
			p := pair{s.Path, earlier}
			if dups[p] == nil {
				order = append(order, p)
			}
			dups[p] = append(dups[p], line)
		}
	}
	for _, p := range order {
		lines := dups[p]
		detail := fmt.Sprintf("%d lines also in %s", len(lines), p.earlier)
		if p.earlier == p.path {
			detail = fmt.Sprintf("%d lines repeated in the same file", len(lines))
		}
		out = append(out, promptFinding{"duplicate", p.path, model(strings.Join(lines, "")), detail})
	}
	slices.SortStableFunc(out, func(a, b promptFinding) int { return cmp.Compare(b.Tokens, a.Tokens) })
	return out
}

func printPrompts(agent, model string, s cost.Saving, perRequest float64, srcs []promptSource, findings []promptFinding) {
	w := os.Stdout
	fmt.Fprintf(w, "%s sends %s tokens of instructions with every request (%s)\n", agent, human(s.ModelTokens), model)
	fmt.Fprintf(w, "%s a request from the cache · %s per session (one write + %.0f reads)\n\n",
		cents(perRequest), cents(s.InContext), cost.AgentContext.Rereads)
	if len(srcs) == 0 {
		fmt.Fprintln(w, "no instruction files found")
		return
	}
	width := 0
	for _, src := range srcs {
		width = max(width, len(src.Path))
	}
	fmt.Fprintf(w, "%-*s %8s  %s\n", width, "source", "tokens", "via")
	for _, src := range srcs {
		fmt.Fprintf(w, "%-*s %8s  %s\n", width, src.Path, human(src.Tokens), src.Via)
	}
	if len(findings) == 0 {
		return
	}
	fmt.Fprintln(w, "\nfindings")
	for _, f := range findings {
		fmt.Fprintf(w, "  %-16s %-*s %8s  %s\n", f.Kind, width, f.Path, human(f.Tokens), f.Detail)
	}
}

// terseFiles prunes instruction prose in place (compress/terse), keeping
// each original as <file>.bak, and reports the tokens saved.
func terseFiles(files []string, ratio float64) error {
	if len(files) == 0 {
		return errors.New("prompts -terse: want files to prune")
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		out := terse.Prune(string(data), min(ratio, 1))
		if out == string(data) {
			fmt.Printf("%s: unchanged\n", f)
			continue
		}
		if err := os.WriteFile(f+".bak", data, 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
			return err
		}
		before, after := tokens.Estimate(string(data)), tokens.Estimate(out)
		fmt.Printf("%s: %d → %d tokens (-%s), original in %s.bak\n", f, before, after, mcptool.Percent(before-after, before), f)
	}
	return nil
}
