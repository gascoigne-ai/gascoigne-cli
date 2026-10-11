// Package testreport compresses test runner output: passing tests collapse
// to counts, failures keep their details, dependency stack frames go,
// summaries stay. Go output is parsed; other runners follow declarative
// dialect rules over one generic engine.
package testreport

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
)

// summary accumulates one go test run.
type summary struct {
	out       []string
	okPkgs    []string
	noTests   int
	passed    int
	tests     map[string]*test // by name, current package only
	order     []string         // failed tests, in report order
	current   string
	inPanic   bool
	stdFrames int
}

type test struct {
	failed bool
	header string   // the "--- FAIL: X (0.00s)" line
	out    []string // output attributed to the test
}

// Go compresses go test output (plain or -v).
func Go(lines []string) []string {
	s := &summary{tests: map[string]*test{}}
	for _, l := range lines {
		s.line(l)
	}
	s.flushPanic()
	s.flushPackage()
	return s.result()
}

func (s *summary) line(l string) {
	trimmed := strings.TrimSpace(l)
	switch {
	case s.inPanic && !isPkgResult(l):
		s.panicLine(l)
	case hasAnyPrefix(trimmed, "=== RUN", "=== PAUSE", "=== CONT", "=== NAME"):
		s.current = strings.TrimSpace(trimmed[len("=== RUN"):])
		s.test(s.current)
	case strings.HasPrefix(trimmed, "--- FAIL: "):
		s.current = testName(trimmed)
		t := s.test(s.current)
		t.failed, t.header = true, l
		s.order = append(s.order, s.current)
	case hasAnyPrefix(trimmed, "--- PASS: ", "--- SKIP: "):
		s.passed++
		s.current = testName(trimmed)
		s.test(s.current)
	case trimmed == "PASS" || trimmed == "FAIL":
	case strings.HasPrefix(l, "ok  \t"):
		s.okPkgs = append(s.okPkgs, strings.Fields(l)[1])
		s.flushPackage()
	case strings.HasPrefix(l, "?   \t"):
		s.noTests++
	case strings.HasPrefix(l, "FAIL\t"):
		s.flushPanic()
		s.flushPackage()
		s.out = append(s.out, l)
	case strings.HasPrefix(l, "panic: "):
		s.inPanic = true
		s.flushPackage()
		s.out = append(s.out, l)
	case s.current != "":
		t := s.tests[s.current]
		t.out = append(t.out, l)
	default:
		s.out = append(s.out, l)
	}
}

func (s *summary) test(name string) *test {
	t, ok := s.tests[name]
	if !ok {
		t = &test{}
		s.tests[name] = t
	}
	return t
}

// flushPackage emits failed tests with their output and forgets the rest.
func (s *summary) flushPackage() {
	for _, name := range s.order {
		t := s.tests[name]
		s.out = append(s.out, t.header)
		s.out = append(s.out, t.out...)
	}
	s.tests, s.order, s.current = map[string]*test{}, nil, ""
}

// panicLine keeps goroutine headers and frames of user code. Frames come in
// pairs: "pkg.Func(args)" then "\t/path/file.go:12 +0x1f".
func (s *summary) panicLine(l string) {
	switch {
	case strings.HasPrefix(l, "\t"):
		if last := len(s.out) - 1; last >= 0 && s.out[last] == "" {
			s.out = s.out[:last] // placeholder of a dropped std frame
			return
		}
		s.out = append(s.out, l)
	case isStdFrame(l):
		s.stdFrames++
		s.out = append(s.out, "") // its file line follows
	default:
		s.out = append(s.out, l)
	}
}

func (s *summary) flushPanic() {
	if s.stdFrames > 0 {
		s.out = append(s.out, compress.Omitted(s.stdFrames, "standard library frames"))
	}
	s.inPanic, s.stdFrames = false, 0
}

func (s *summary) result() []string {
	out := compress.SqueezeBlank(s.out)
	if n := len(s.okPkgs); n > 0 {
		prefix := commonPrefix(s.okPkgs)
		names := make([]string, n)
		for i, p := range s.okPkgs {
			names[i] = cmp.Or(strings.TrimPrefix(p, prefix), p)
		}
		line := "ok " + plural(n, "package")
		if prefix != "" {
			line += " in " + strings.TrimSuffix(prefix, "/")
		}
		out = append(out, line+": "+strings.Join(names, ", "))
	}
	if s.noTests > 0 {
		out = append(out, plural(s.noTests, "package")+" without tests")
	}
	if s.passed > 0 {
		out = append(out, plural(s.passed, "test")+" passed")
	}
	return out
}

func isPkgResult(l string) bool {
	return hasAnyPrefix(l, "FAIL\t", "ok  \t")
}

// isStdFrame reports a stack frame of the standard library: by convention its
// import path has no dot in the first element ("testing.tRunner", not "example.com/x.F").
func isStdFrame(l string) bool {
	if strings.HasPrefix(l, "goroutine ") {
		return false
	}
	fn := strings.TrimPrefix(l, "created by ")
	first, _, _ := strings.Cut(fn, "/")
	if strings.Contains(fn, "/") {
		return !strings.Contains(first, ".")
	}
	return strings.Contains(fn, "(") || strings.HasPrefix(l, "created by ")
}

func testName(trimmed string) string {
	f := strings.Fields(trimmed)
	if len(f) < 3 {
		return ""
	}
	return f[2]
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(s, p) })
}

// commonPrefix returns the longest shared prefix ending in "/".
func commonPrefix(paths []string) string {
	if len(paths) < 2 {
		return ""
	}
	p := paths[0]
	for _, q := range paths[1:] {
		for !strings.HasPrefix(q, p) {
			p = p[:len(p)-1]
		}
	}
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[:i+1]
	}
	return ""
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
