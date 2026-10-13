package docoptimizer

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

const (
	// The model loads on the helper's first start: seconds on a GPU, longer cold.
	helperStartTimeout = 60 * time.Second
	helperCallTimeout  = 20 * time.Second
	// A helper failing this often is given up on for the process lifetime.
	helperMaxFailures = 3
)

var errHelperGone = errors.New("prose helper given up")

// helper talks to the `gascoigne prose serve` process: one JSON line per request,
// one per reply, after a "ready" line once the model is loaded. Requests are
// serialized because the server calls the optimizer concurrently.
type helper struct {
	cfg *Model
	log *slog.Logger

	mu       sync.Mutex
	cmd      *exec.Cmd
	stdin    interface{ Write([]byte) (int, error) }
	stdout   *bufio.Reader
	stderr   *tail
	failures int
}

type request struct {
	Words     []string `json:"words"`
	Threshold float64  `json:"threshold"`
}

type response struct {
	Keep []bool `json:"keep"`
}

// keep reports, per word, whether the model keeps it.
func (h *helper) keep(words []string) ([]bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.failures >= helperMaxFailures {
		return nil, errHelperGone
	}
	if h.cmd == nil {
		if err := h.start(); err != nil {
			return nil, h.fail(err)
		}
	}
	var res response
	if err := h.call(request{words, h.cfg.Threshold}, &res); err != nil {
		return nil, h.fail(err)
	}
	if len(res.Keep) != len(words) {
		return nil, h.fail(fmt.Errorf("helper returned %d flags for %d words", len(res.Keep), len(words)))
	}
	h.failures = 0
	return res.Keep, nil
}

// warm starts the helper ahead of the first request: the model load takes
// seconds, and a request arriving meanwhile waits for it on the mutex.
func (h *helper) warm() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cmd != nil || h.failures >= helperMaxFailures {
		return
	}
	if err := h.start(); err != nil {
		h.fail(err)
	}
}

// argv is the configured command, else this binary's `prose serve`.
func (h *helper) argv() ([]string, error) {
	if len(h.cfg.Command) > 0 {
		return h.cfg.Command, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("find own binary: %w", err)
	}
	return []string{exe, "prose", "serve"}, nil
}

func (h *helper) start() error {
	args, err := h.argv()
	if err != nil {
		return err
	}
	cmd := exec.Command(args[0], args[1:]...)
	h.stderr = &tail{}
	cmd.Stderr = h.stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", args[0], err)
	}
	h.cmd, h.stdin, h.stdout = cmd, in, bufio.NewReader(out)
	var ready string
	if err := h.within(helperStartTimeout, func() (err error) { ready, err = h.stdout.ReadString('\n'); return }); err != nil {
		return fmt.Errorf("wait for %s: %w", args[0], err)
	}
	device, ok := strings.CutPrefix(strings.TrimSpace(ready), "ready")
	if !ok {
		return fmt.Errorf("unexpected helper greeting %q", strings.TrimSpace(ready))
	}
	h.log.Debug("prose helper started", "device", strings.TrimSpace(device))
	return nil
}

func (h *helper) call(req request, res *response) error {
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	return h.within(helperCallTimeout, func() error {
		if _, err := h.stdin.Write(append(line, '\n')); err != nil {
			return err
		}
		reply, err := h.stdout.ReadBytes('\n')
		if err != nil {
			return err
		}
		return json.Unmarshal(reply, res)
	})
}

// within runs fn and kills the helper when it takes longer than d, which
// unblocks the pipe read.
func (h *helper) within(d time.Duration, fn func() error) error {
	proc := h.cmd.Process
	t := time.AfterFunc(d, func() { proc.Kill() })
	defer t.Stop()
	return fn()
}

// fail drops the helper process so the next call restarts it, and warns once.
func (h *helper) fail(err error) error {
	if h.stderr != nil {
		if msg := h.stderr.String(); msg != "" {
			err = fmt.Errorf("%w: %s", err, msg)
		}
	}
	h.stop()
	if h.failures++; h.failures == helperMaxFailures {
		h.log.Warn("prose helper unavailable, prose compression off", "err", err)
	} else {
		h.log.Debug("prose helper failed", "err", err)
	}
	return err
}

func (h *helper) stop() {
	if h.cmd == nil {
		return
	}
	h.cmd.Process.Kill()
	h.cmd.Wait()
	h.cmd = nil
}

// tail keeps the last bytes the helper wrote to stderr: its errors, which
// would otherwise be lost.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

const tailSize = 1024

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > tailSize {
		t.buf = t.buf[len(t.buf)-tailSize:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(string(t.buf))
}

func (h *helper) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stop()
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
	if len(words) < o.cfg.Model.MinWords || tabular(lines) {
		return lines
	}
	keep, err := o.helper.keep(words)
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
