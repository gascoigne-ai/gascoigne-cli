// Package diag compresses compiler, linter and bundler output into
// diagnostics grouped by file: paths printed once, repeats folded, code
// frames capped, tool noise (progress, asset listings, downloads) dropped.
// Lines that are neither diagnostics nor noise (summaries, unknown errors)
// are kept.
package diag

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// Diagnostic is one finding.
type Diagnostic struct {
	File     string
	Line     int
	Col      int
	Severity string // error, warning, note…; may be empty
	Code     string // TS2322, E501, CS0103…
	Message  string
	Detail   []string // "symbol: variable X" lines javac prints under the message
	Context  []string // code frame lines after it
}

var (
	// file(l,c): error TS2322: msg [project]   (tsc, msbuild/dotnet)
	// also msvc "a.cpp(12): error C2065: msg" and dmd "a.d(12,5): Error: msg": column and code are optional
	parenRe = lazyre.New(`^\s*(\S+?)\((\d+)(?:,(\d+))?\): (error|warning|info|Error|Warning|Deprecation|fatal error)(?: ([A-Za-z]+\d+))?:\s+(.+?)(?: \[[^\]]+\])?$`)
	// file:l:c: [severity:] msg   (go, gcc/clang, golangci, ruff, mypy, javac, eslint unix)
	colonRe = lazyre.New(`^(?:\./)?([^\s:()][^:()]*?\.[A-Za-z0-9]+):(\d+)(?::(\d+))?:\s*(?:(fatal error|error|warning|note|info|hint)\s*:?\s*)?(?:(\[?[A-Z]+-?\d+\]?)\s+)?(.+)$`)
	// rust/cargo: "error[E0308]: msg" then "  --> src/main.rs:4:5"
	rustHeadRe = lazyre.New(`^(error|warning)(?:\[(E\d+)\])?: (.+)$`)
	// ruff full format: "F401 [*] msg" then " --> file:l:c"
	ruffHeadRe = lazyre.New(`^([A-Z]+\d+) (?:\[\*\] )?(.+)$`)
	rustLocRe  = lazyre.New(`^\s*--> (\S+?):(\d+):(\d+)$`)
	// eslint stylish: path line, then "  12:5  error  msg  rule"
	eslintRowRe = lazyre.New(`^\s+(\d+):(\d+)\s+(error|warning)\s+(.+?)\s{2,}(\S+)$`)
	eslintFile  = lazyre.New(`^(/|\./|[A-Za-z]:\\)?\S+\.[jt]sx?$|^\S+\.(vue|svelte|mjs|cjs|css|scss|sass|less|ya?ml|md)$`)
	noteRe      = lazyre.New(`^(recursively )?required (from|by)\b`)
	// dart analyze: "  error - lib/main.dart:12:5 - msg - code"
	dartRe = lazyre.New(`^\s+(error|warning|info) [-•] (\S+?):(\d+):(\d+) [-•] (.+?) [-•] (\w+)$`)
	// kotlinc: "e: file:///src/Main.kt:12:5 Unresolved reference: x"
	kotlinRe = lazyre.New(`^([ew]): (?:file://)?(\S+\.kts?):(\d+):(\d+)\s+(.+)$`)
	// elixir: header "warning: unused variable x", location "  lib/foo.ex:12: Foo.bar/1"
	elixirLocRe = lazyre.New(`^\s+(\S+\.exs?):(\d+)(?::(\d+))?: \S+$`)
	// [ERROR] /path/File.java:[14,20] msg   (maven compiler plugin)
	mavenRe = lazyre.New(`^\[(ERROR|WARNING)\] (\S+\.(?:java|kt|scala)):\[(\d+),(\d+)\] (.+)$`)
	// [ERROR]   symbol:   variable X   (maven: lines under a compiler error)
	mavenDetailRe = lazyre.New(`^\[(?:ERROR|WARNING)\]\s{2,}(symbol|location|required|found|reason|expected|actual):\s*(.+)$`)
	// code frame lines: indented, gutters, carets.
	frameRe = lazyre.New(`^(\s{2,}|\s*\d+\s*\||\s*\|)|^\s*\^|^\s*~+\s*$`)
)

// Rules tune the engine for a dialect.
type Rules struct {
	Strip    *lazyre.Regexp // prefix removed from every line first (docker "#8 1.234 ")
	Tag      *lazyre.Regexp // prefix whose group 1 is the severity of a diagnostic without one (sbt "[error] ")
	Noise    *lazyre.Regexp // dropped lines
	MaxFrame int            // code frame lines kept per diagnostic
}

// Compress parses lines into diagnostics and renders them grouped by file.
func Compress(lines []string, r Rules) []string {
	if r.MaxFrame == 0 {
		r.MaxFrame = 4
	}
	lines = collapse(lines, 2)
	var (
		diags   []*Diagnostic
		other   []string // kept non-diagnostic lines, in order, as markers into diags
		order   []any
		cur     *Diagnostic
		pending *Diagnostic // rust header waiting for its --> location
		esFile  string
		dropped int
	)
	add := func(d *Diagnostic) {
		diags = append(diags, d)
		order = append(order, d)
		cur = d
	}
	keep := func(t string) {
		other = append(other, t)
		order = append(order, len(other)-1)
	}
	// flushPending keeps a header that never got a location as plain text.
	flushPending := func() {
		if pending != nil {
			keep(strings.TrimPrefix(strings.Join([]string{pending.Severity, pending.Code}, " ")+": ", " ") + pending.Message)
			pending = nil
		}
	}
	for _, l := range lines {
		t := strings.TrimRight(l, " \t")
		if r.Strip != nil {
			t = r.Strip.ReplaceAllString(t, "")
		}
		tag := ""
		if r.Tag != nil {
			if m := r.Tag.FindStringSubmatch(t); m != nil {
				// Only diagnostics and their frames lose the prefix; other lines stay as printed.
				if rest := t[len(m[0]):]; match(rest) != nil || cur != nil && frameRe.MatchString(rest) {
					tag, t = tagSeverity(m[1]), rest
				}
			}
		}
		switch m := match(t); {
		case r.Noise != nil && r.Noise.MatchString(t):
			dropped++
		case m != nil:
			if m.Severity == "" {
				m.Severity = tag
			}
			add(m)
			pending, esFile = nil, ""
		case rustHeadRe.MatchString(t):
			flushPending()
			s := rustHeadRe.FindStringSubmatch(t)
			pending = &Diagnostic{Severity: s[1], Code: s[2], Message: s[3]}
			cur = nil
		case ruffHeadRe.MatchString(t):
			flushPending()
			s := ruffHeadRe.FindStringSubmatch(t)
			pending = &Diagnostic{Code: s[1], Message: s[2]}
			cur = nil
		case pending != nil && (rustLocRe.MatchString(t) || elixirLocRe.MatchString(t)):
			s := rustLocRe.FindStringSubmatch(t)
			if s == nil {
				s = elixirLocRe.FindStringSubmatch(t)
			}
			pending.File, pending.Line, pending.Col = s[1], atoi(s[2]), atoi(s[3])
			add(pending)
			pending = nil
		case eslintFile.MatchString(strings.TrimSpace(t)) && !strings.HasPrefix(t, " "):
			esFile, cur = strings.TrimSpace(t), nil
		case esFile != "" && eslintRowRe.MatchString(t):
			s := eslintRowRe.FindStringSubmatch(t)
			add(&Diagnostic{File: esFile, Line: atoi(s[1]), Col: atoi(s[2]), Severity: s[3], Message: s[4], Code: s[5]})
		case cur != nil && mavenDetailRe.MatchString(t):
			s := mavenDetailRe.FindStringSubmatch(t)
			cur.Detail = append(cur.Detail, s[1]+": "+strings.TrimSpace(s[2]))
		case cur != nil && t != "" && frameRe.MatchString(t):
			cur.Context = append(cur.Context, t)
		case t == "":
			cur = nil
		default:
			cur = nil
			flushPending()
			keep(t)
		}
	}
	flushPending()
	if len(diags) == 0 {
		return compress.SqueezeBlank(other)
	}
	return render(diags, other, order, r.MaxFrame)
}

func match(t string) *Diagnostic {
	if s := mavenRe.FindStringSubmatch(t); s != nil {
		return &Diagnostic{File: s[2], Line: atoi(s[3]), Col: atoi(s[4]), Severity: strings.ToLower(s[1]), Message: s[5]}
	}
	if s := dartRe.FindStringSubmatch(t); s != nil {
		return &Diagnostic{File: s[2], Line: atoi(s[3]), Col: atoi(s[4]), Severity: s[1], Code: s[6], Message: s[5]}
	}
	if s := kotlinRe.FindStringSubmatch(t); s != nil {
		return &Diagnostic{File: s[2], Line: atoi(s[3]), Col: atoi(s[4]), Severity: map[string]string{"e": "error", "w": "warning"}[s[1]], Message: s[5]}
	}
	if s := parenRe.FindStringSubmatch(t); s != nil {
		return &Diagnostic{File: s[1], Line: atoi(s[2]), Col: atoi(s[3]), Severity: strings.ToLower(s[4]), Code: s[5], Message: s[6]}
	}
	if s := colonRe.FindStringSubmatch(t); s != nil && !strings.Contains(s[1], " ") {
		d := &Diagnostic{File: s[1], Line: atoi(s[2]), Col: atoi(s[3]), Severity: s[4], Code: strings.Trim(s[5], "[]"), Message: s[6]}
		if d.Severity == "" && noteRe.MatchString(d.Message) {
			d.Severity = "note" // gcc's instantiation chain: "required from here"
		}
		return d
	}
	return nil
}

// tagSeverity names the level of a tool's own prefix: sbt says warn.
func tagSeverity(s string) string {
	if s == "warn" {
		return "warning"
	}
	return s
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

// render prints kept text lines where they occurred and all diagnostics
// grouped by file at the position of the first one, then a count summary.
func render(diags []*Diagnostic, other []string, order []any, maxFrame int) []string {
	byFile := map[string][]*Diagnostic{}
	var files []string
	for _, d := range diags {
		if _, ok := byFile[d.File]; !ok {
			files = append(files, d.File)
		}
		byFile[d.File] = append(byFile[d.File], d)
	}
	var out []string
	printed := false
	for _, o := range order {
		switch o := o.(type) {
		case int:
			out = append(out, other[o])
		case *Diagnostic:
			if printed {
				continue
			}
			printed = true
			shared := sharedAcross(byFile, files)
			for _, f := range files {
				ds := slices.DeleteFunc(slices.Clone(byFile[f]), func(d *Diagnostic) bool { return shared[sig(d)] != nil })
				if len(ds) == 0 {
					continue
				}
				out = append(out, f)
				out = append(out, fileLines(ds, maxFrame)...)
			}
			out = append(out, sharedLines(shared, maxFrame)...)
		}
	}
	out = compress.SqueezeBlank(out)
	return append(out, summary(diags, len(files)))
}

// maxAlso is how many extra lines a repeated finding lists before "… N more".
const maxAlso = 11

func fileLines(ds []*Diagnostic, maxFrame int) []string {
	lines := map[signature][]int{}
	for _, d := range ds {
		lines[sig(d)] = append(lines[sig(d)], d.Line)
	}
	seen := map[signature]bool{}
	var out []string
	for _, d := range ds {
		k := sig(d)
		if seen[k] {
			continue
		}
		seen[k] = true
		pos := strconv.Itoa(d.Line)
		if d.Col > 0 {
			pos += ":" + strconv.Itoa(d.Col)
		}
		head := strings.Join(slices.DeleteFunc([]string{"  " + pos, d.Severity, d.Code}, func(s string) bool { return s == "" }), " ")
		line := head + " " + text(d)
		if n := len(lines[k]); n > 1 {
			line += repeats(n, d.Line, lines[k])
		}
		out = append(out, line)
		ctx := capFrame(d.Context, maxFrame)
		for _, c := range ctx {
			out = append(out, "    "+strings.TrimLeft(c, " "))
		}
	}
	return out
}

// text is the message with the detail lines the compiler printed under it.
func text(d *Diagnostic) string {
	if len(d.Detail) == 0 {
		return d.Message
	}
	return d.Message + " [" + strings.Join(d.Detail, "; ") + "]"
}

// repeats says a finding came n times: at which other lines, or "same place"
// when a tool printed it twice (maven and dotnet repeat errors in a summary).
func repeats(n, first int, lines []int) string {
	other := slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(lines))), func(l int) bool { return l == first })
	if len(other) == 0 {
		return fmt.Sprintf(" (×%d, same place)", n)
	}
	return fmt.Sprintf(" (×%d, also at %s)", n, alsoAt(other))
}

// alsoAt lists lines of a repeated finding, the first maxAlso of them.
func alsoAt(lines []int) string {
	parts := make([]string, 0, maxAlso+1)
	for _, l := range lines[:min(len(lines), maxAlso)] {
		parts = append(parts, strconv.Itoa(l))
	}
	if more := len(lines) - maxAlso; more > 0 {
		parts = append(parts, fmt.Sprintf("… %d more", more))
	}
	return strings.Join(parts, ", ")
}

func summary(diags []*Diagnostic, files int) string {
	sev := map[string]int{}
	for _, d := range diags {
		sev[cmp.Or(d.Severity, "error")]++
	}
	var parts []string
	for _, s := range []string{"fatal error", "error", "warning", "note", "info", "hint"} {
		if n := sev[s]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, plural(s, n)))
		}
	}
	return fmt.Sprintf("%s in %d %s", strings.Join(parts, ", "), files, plural("file", files))
}

func plural(s string, n int) string {
	if n == 1 {
		return s
	}
	return s + "s"
}

// annotated frame lines point at the problem: carets, labels, help and notes.
var annotated = lazyre.New(`\^|~~|-{3,}|\bhelp:|\bnote:|= note|= help`)

// capFrame keeps at most max lines, preferring annotated lines and the
// source line right above each.
func capFrame(ctx []string, max int) []string {
	if len(ctx) <= max {
		return ctx
	}
	keep := make([]bool, len(ctx))
	n := 0
	for i, l := range ctx {
		if annotated.MatchString(l) && n < 2*max {
			keep[i] = true
			n++
			if i > 0 && !keep[i-1] {
				keep[i-1] = true
				n++
			}
		}
	}
	var out []string
	for i, l := range ctx {
		if keep[i] {
			out = append(out, l)
		}
	}
	if len(out) == 0 {
		out = ctx[:max]
	}
	if dropped := len(ctx) - len(out); dropped > 0 {
		out = append(out, "    "+compress.Omitted(dropped, "frame lines"))
	}
	return out
}

type signature struct{ sev, code, msg string }

func sig(d *Diagnostic) signature { return signature{d.Severity, d.Code, text(d)} }

// shared is a diagnostic repeated in several files, printed once.
type shared struct {
	first *Diagnostic
	where []string // file:line:col
}

// sharedAcross finds diagnostics identical in at least 3 files.
func sharedAcross(byFile map[string][]*Diagnostic, files []string) map[signature]*shared {
	found := map[signature]*shared{}
	inFiles := map[signature]map[string]bool{}
	for _, f := range files {
		for _, d := range byFile[f] {
			s := sig(d)
			if inFiles[s] == nil {
				inFiles[s] = map[string]bool{}
				found[s] = &shared{first: d}
			}
			inFiles[s][f] = true
			found[s].where = append(found[s].where, fmt.Sprintf("%s:%d", f, d.Line))
		}
	}
	for s, fs := range inFiles {
		if len(fs) < 3 {
			delete(found, s)
		}
	}
	return found
}

func sharedLines(found map[signature]*shared, maxFrame int) []string {
	if len(found) == 0 {
		return nil
	}
	list := slices.Collect(maps.Values(found))
	slices.SortFunc(list, func(a, b *shared) int {
		return cmp.Or(cmp.Compare(a.where[0], b.where[0]), cmp.Compare(a.first.Line, b.first.Line),
			cmp.Compare(a.first.Code, b.first.Code), cmp.Compare(a.first.Message, b.first.Message))
	})
	out := []string{"same in several files:"}
	for _, s := range list {
		d := s.first
		head := strings.Join(slices.DeleteFunc([]string{"  " + cmp.Or(d.Severity, "error"), d.Code}, func(s string) bool { return s == "" }), " ")
		out = append(out, head+" "+text(d))
		out = append(out, "    at "+strings.Join(s.where, ", "))
		for _, c := range capFrame(d.Context, maxFrame) {
			out = append(out, "    "+strings.TrimLeft(c, " "))
		}
	}
	return out
}
