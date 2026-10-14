package proseoptimizer

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/prose"
)

// maxFailures: a model failing this often is given up on for the process lifetime.
const maxFailures = 3

var errGivenUp = fmt.Errorf("prose model given up")

// model is what the optimizer needs of the loaded model (prose.Model).
type model interface {
	Keep(words []string, threshold float64) ([]bool, error)
	Device() string
	Close()
}

// loaded holds the model, opened on first use or by warm. Failing to open or
// run it never fails a command: the paragraph stays as it was.
type loaded struct {
	cfg  *Config
	open func() (model, error)
	log  *slog.Logger

	mu       sync.Mutex
	m        model
	failures int
	last     time.Time   // the model's last use
	idle     *time.Timer // unloads the model cfg.Idle after last
}

// openModel checks the files against their pinned hashes before the first
// load, so a damaged download fails with a clear error instead of inside the
// runtime. Later loads (after an idle unload) skip the hash.
func openModel(cfg *Config) func() (model, error) {
	verified := false
	return func() (model, error) {
		dir, err := prose.ModelDir()
		if err != nil {
			return nil, err
		}
		if !verified {
			if err := prose.Verify(dir, cfg.Model); err != nil {
				return nil, fmt.Errorf("%w (gascoigne prose install fetches it again)", err)
			}
			verified = true
		}
		return prose.Open(dir, "", cfg.Device, cfg.Model)
	}
}

// keep reports, per word, whether the model keeps it.
func (l *loaded) keep(words []string) (keep []bool, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failures >= maxFailures {
		return nil, errGivenUp
	}
	defer func() {
		if r := recover(); r != nil { // a native runtime fault must not take the server down with it
			keep, err = nil, l.fail(fmt.Errorf("model panicked: %v", r))
		}
	}()
	if l.m == nil {
		if err := l.load(); err != nil {
			return nil, l.fail(err)
		}
	}
	keep, err = l.m.Keep(words, l.cfg.Threshold)
	if err == nil && len(keep) != len(words) {
		err = fmt.Errorf("model returned %d flags for %d words", len(keep), len(words))
	}
	if err != nil {
		return nil, l.fail(err)
	}
	l.failures = 0
	l.touch()
	return keep, nil
}

func (l *loaded) load() error {
	m, err := l.open()
	if err != nil {
		return fmt.Errorf("load prose model (gascoigne prose install?): %w", err)
	}
	l.m = m
	l.log.Debug("prose model loaded", "device", m.Device())
	l.touch()
	return nil
}

// touch marks the model used and arms the idle unload. The caller holds mu.
func (l *loaded) touch() {
	l.last = time.Now()
	if l.cfg.Idle <= 0 || l.idle != nil {
		return
	}
	l.idle = time.AfterFunc(l.cfg.Idle, l.unloadIfIdle)
}

// unloadIfIdle frees the model once it has sat unused for cfg.Idle; if it was
// used meanwhile, it waits out the rest.
func (l *loaded) unloadIfIdle() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.idle = nil
	if l.m == nil {
		return
	}
	if rest := l.cfg.Idle - time.Since(l.last); rest > 0 {
		l.idle = time.AfterFunc(rest, l.unloadIfIdle)
		return
	}
	l.m.Close()
	l.m = nil
	l.log.Debug("prose model unloaded", "idle", l.cfg.Idle)
}

// warm loads the model ahead of the first request: it takes seconds.
func (l *loaded) warm() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.m != nil || l.failures >= maxFailures {
		return
	}
	if err := l.load(); err != nil {
		l.fail(err)
	}
}

// fail counts a failure, drops the model so the next call reloads it, and
// warns once when it is given up.
func (l *loaded) fail(err error) error {
	if l.m != nil {
		l.m.Close()
		l.m = nil
	}
	if l.failures++; l.failures == maxFailures {
		l.log.Warn("prose model unavailable, prose compression off", "err", err)
	} else {
		l.log.Debug("prose model failed", "err", err)
	}
	return err
}

func (l *loaded) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.idle != nil {
		l.idle.Stop()
		l.idle = nil
	}
	if l.m != nil {
		l.m.Close()
		l.m = nil
	}
}

// structural lines (headings, tables, quotes, lists, HTML, links, setext
// underlines, gascoigne's own omission markers) keep their exact text and
// end the paragraph before them.
var structural = lazyre.New(`^(\s|#|\||>|[-*+] |\d+[.)] |<|!?\[|=+$|-+$|…|\[gascoigne)`)

// prose compresses the plain paragraphs outside fenced code and leaves
// everything else as is. A helper failure leaves the lines as they were.
func (o *Optimizer) prose(lines []string) []string {
	var out, para []string
	inCode := false
	flush := func() {
		out = append(out, o.paragraph(para)...)
		para = nil
	}
	for _, l := range lines {
		switch {
		case fence.MatchString(l):
			flush()
			inCode = !inCode
			out = append(out, l)
		case inCode || strings.TrimSpace(l) == "" || structural.MatchString(l):
			flush()
			out = append(out, l)
		default:
			para = append(para, l)
		}
	}
	flush()
	return out
}

func (o *Optimizer) paragraph(lines []string) []string {
	var words []string
	var lineOf []int
	for i, l := range lines {
		for w := range strings.FieldsSeq(l) {
			words, lineOf = append(words, w), append(lineOf, i)
		}
	}
	if len(words) < o.cfg.MinWords || tabular(lines) {
		return lines
	}
	keep, err := o.model.keep(words)
	if err != nil {
		return lines
	}
	kept := make([][]string, len(lines))
	for i, w := range words {
		if keep[i] || mustKeep(w) {
			kept[lineOf[i]] = append(kept[lineOf[i]], w)
		}
	}
	var out []string
	for _, ws := range kept {
		if len(ws) > 0 {
			out = append(out, strings.Join(ws, " "))
		}
	}
	return out
}

// mustKeep mirrors the labeller's hard-keep overlay the model was trained
// with (identifiers, numbers, paths, flags, negations and connectives), plus
// markdown inline code and links, which the overlay has no class for.
// Lookarounds of the original are approximated with boundary classes.
var mustKeepRe = lazyre.New(`\b0x[0-9A-Fa-f]+\b` +
	`|(?:^|[^\w.])\d+(?:\.\d+)?(?:$|[^\w.])` +
	`|[A-Z_]{2,}` +
	`|[a-z_][a-z0-9_]*\.[a-z0-9_]+` +
	`|/[a-z0-9/._-]{2,}` +
	`|\.[a-z]{2,4}\b` +
	`|--?[a-z][\w-]*` +
	`|\b[A-Z][a-z]+[A-Z]\w*` +
	`|(?i:\b(?:not|never|none|cannot|can't|don't|doesn't|didn't|won't|shouldn't` +
	`|mustn't|isn't|aren't|avoid|refuse|prohibited|forbidden|disallow|unless` +
	`|except|without|must|should|shall|required|always|only|mandatory)\b)` +
	`|(?i:\b(?:and|or|nor|xor)\b)` +
	"|`|\\]\\(|://|@")

func mustKeep(w string) bool { return mustKeepRe.MatchString(w) }

var columnGap = lazyre.New(`\s{2,}`)

// tabular reports a run of 3+ lines with the same count of 3+ columns
// (ls -l, ps, df): command output is catalogued, not prose, and dropping
// words row by row would misalign it. Text of unknown type lands here too.
func tabular(lines []string) bool {
	run, prev := 0, 0
	for _, l := range lines {
		cols := len(columnGap.Split(strings.TrimSpace(l), -1))
		if cols < 3 || cols != prev {
			run = 0
		}
		if run++; run >= 3 && cols >= 3 {
			return true
		}
		prev = cols
	}
	return false
}
