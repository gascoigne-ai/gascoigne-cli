package prose

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const barWidth = 30

// progress copies a download and draws a bar on w as it goes.
type progress struct {
	w           io.Writer
	total, done int64
	last        time.Time
}

// stderrIsTerminal says whether a bar can be redrawn in place; piped or
// background output gets no bar, which would only fill a log with carriage returns.
func stderrIsTerminal() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// copyWithBar is io.Copy that draws a bar on stderr when it is a terminal;
// total is the expected size, or -1 when unknown.
func copyWithBar(dst io.Writer, src io.Reader, total int64) (int64, error) {
	if !stderrIsTerminal() {
		return io.Copy(dst, src)
	}
	p := &progress{w: os.Stderr, total: total}
	n, err := io.Copy(io.MultiWriter(dst, p), src)
	p.draw(true)
	return n, err
}

func (p *progress) Write(b []byte) (int, error) {
	p.done += int64(len(b))
	p.draw(false)
	return len(b), nil
}

// draw redraws at most ten times a second, unless final.
func (p *progress) draw(final bool) {
	if !final && time.Since(p.last) < 100*time.Millisecond {
		return
	}
	p.last = time.Now()
	fmt.Fprint(p.w, "\r  "+p.line())
	if final {
		fmt.Fprintln(p.w)
	}
}

func (p *progress) line() string {
	const mb = 1 << 20
	if p.total <= 0 {
		return fmt.Sprintf("%d MB", p.done/mb)
	}
	frac := min(float64(p.done)/float64(p.total), 1)
	fill := int(frac * barWidth)
	return fmt.Sprintf("[%s%s] %3d%%  %d/%d MB", strings.Repeat("#", fill), strings.Repeat("-", barWidth-fill),
		int(frac*100), p.done/mb, p.total/mb)
}
