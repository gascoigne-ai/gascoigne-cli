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

var cached sync.Map // expr -> *Regexp

// Cached returns expr compiled on first use, one shared Regexp per distinct
// expr. Use it for patterns that arrive as data (language rows) so repeated
// calls do not register a new pattern each time; New is for package-level
// patterns.
func Cached(expr string) *Regexp {
	if re, ok := cached.Load(expr); ok {
		return re.(*Regexp)
	}
	re, _ := cached.LoadOrStore(expr, New(expr))
	return re.(*Regexp)
}

// Count is the number of patterns made with New so far.
func Count() int {
	mu.Lock()
	defer mu.Unlock()
	return len(all)
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

func (r *Regexp) String() string                          { return r.expr }
func (r *Regexp) MatchString(text string) bool            { return r.get().MatchString(text) }
func (r *Regexp) FindStringIndex(text string) []int       { return r.get().FindStringIndex(text) }
func (r *Regexp) FindStringSubmatch(text string) []string { return r.get().FindStringSubmatch(text) }
func (r *Regexp) FindStringSubmatchIndex(text string) []int {
	return r.get().FindStringSubmatchIndex(text)
}
func (r *Regexp) FindAllString(text string, limit int) []string {
	return r.get().FindAllString(text, limit)
}
func (r *Regexp) FindAllStringIndex(text string, limit int) [][]int {
	return r.get().FindAllStringIndex(text, limit)
}
func (r *Regexp) FindAllStringSubmatch(text string, limit int) [][]string {
	return r.get().FindAllStringSubmatch(text, limit)
}
func (r *Regexp) FindAllStringSubmatchIndex(text string, limit int) [][]int {
	return r.get().FindAllStringSubmatchIndex(text, limit)
}
func (r *Regexp) ReplaceAllString(src, repl string) string {
	return r.get().ReplaceAllString(src, repl)
}
func (r *Regexp) ReplaceAllStringFunc(src string, repl func(string) string) string {
	return r.get().ReplaceAllStringFunc(src, repl)
}
func (r *Regexp) Split(text string, limit int) []string { return r.get().Split(text, limit) }
func (r *Regexp) Match(data []byte) bool                { return r.get().Match(data) }
