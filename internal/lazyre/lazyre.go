// Package lazyre compiles regular expressions on first use. Package-level
// patterns compiled at init cost every gascoigne process ~2 ms before main
// runs, even a CLI run that hands its payload to the background server and
// never matches anything.
package lazyre

import (
	"errors"
	"fmt"
	"regexp"
	"sync"
)

// Regexp is a regexp.Regexp compiled the first time a method needs it.
// A bad pattern panics then, like regexp.MustCompile at init; Check finds
// them in tests.
type Regexp struct {
	expr string
	get  func() *regexp.Regexp
}

var (
	mu  sync.Mutex
	all []*Regexp
)

// New returns expr compiled on first use. Call it for package-level
// patterns only: each one stays registered for Check.
func New(expr string) *Regexp {
	r := &Regexp{expr: expr, get: sync.OnceValue(func() *regexp.Regexp { return regexp.MustCompile(expr) })}
	mu.Lock()
	all = append(all, r)
	mu.Unlock()
	return r
}

// Check compiles every pattern made with New so far.
func Check() error {
	mu.Lock()
	defer mu.Unlock()
	var errs []error
	for _, r := range all {
		if _, err := regexp.Compile(r.expr); err != nil {
			errs = append(errs, fmt.Errorf("%q: %w", r.expr, err))
		}
	}
	return errors.Join(errs...)
}

// Regexp is the compiled expression.
func (r *Regexp) Regexp() *regexp.Regexp { return r.get() }

func (r *Regexp) String() string                       { return r.expr }
func (r *Regexp) MatchString(s string) bool            { return r.get().MatchString(s) }
func (r *Regexp) FindString(s string) string           { return r.get().FindString(s) }
func (r *Regexp) FindStringIndex(s string) []int       { return r.get().FindStringIndex(s) }
func (r *Regexp) FindStringSubmatch(s string) []string { return r.get().FindStringSubmatch(s) }
func (r *Regexp) FindStringSubmatchIndex(s string) []int {
	return r.get().FindStringSubmatchIndex(s)
}
func (r *Regexp) FindAllString(s string, n int) []string { return r.get().FindAllString(s, n) }
func (r *Regexp) FindAllStringIndex(s string, n int) [][]int {
	return r.get().FindAllStringIndex(s, n)
}
func (r *Regexp) FindAllStringSubmatch(s string, n int) [][]string {
	return r.get().FindAllStringSubmatch(s, n)
}
func (r *Regexp) FindAllStringSubmatchIndex(s string, n int) [][]int {
	return r.get().FindAllStringSubmatchIndex(s, n)
}
func (r *Regexp) ReplaceAllString(src, repl string) string {
	return r.get().ReplaceAllString(src, repl)
}
func (r *Regexp) ReplaceAllLiteralString(src, repl string) string {
	return r.get().ReplaceAllLiteralString(src, repl)
}
func (r *Regexp) ReplaceAllStringFunc(src string, repl func(string) string) string {
	return r.get().ReplaceAllStringFunc(src, repl)
}
func (r *Regexp) Split(s string, n int) []string { return r.get().Split(s, n) }
func (r *Regexp) SubexpNames() []string          { return r.get().SubexpNames() }
func (r *Regexp) SubexpIndex(name string) int    { return r.get().SubexpIndex(name) }
func (r *Regexp) NumSubexp() int                 { return r.get().NumSubexp() }
func (r *Regexp) Match(b []byte) bool            { return r.get().Match(b) }
