package proseoptimizer

import (
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// fake drops every word starting with "drop", as the model would.
type fake struct{ closed atomic.Bool }

func (f *fake) Keep(words []string, _ float64) ([]bool, error) {
	keep := make([]bool, len(words))
	for i, w := range words {
		keep[i] = !strings.HasPrefix(w, "drop")
	}
	return keep, nil
}
func (f *fake) Device() string { return "fake" }
func (f *fake) Close()         { f.closed.Store(true) }

// short answers one flag too few.
type short struct{ fake }

func (*short) Keep([]string, float64) ([]bool, error) { return []bool{true}, nil }

type opening struct {
	calls atomic.Int32
	m     model
	err   error
}

func (o *opening) open() (model, error) {
	o.calls.Add(1)
	return o.m, o.err
}

func proseOptimizer(t *testing.T, o *opening) *Optimizer {
	cfg := &Config{Enabled: true, Device: "auto", Threshold: 0.5, MinWords: 4}
	return &Optimizer{cfg: cfg, model: &loaded{cfg: cfg, open: o.open, log: slog.New(slog.DiscardHandler)}}
}

func working(t *testing.T) *Optimizer { return proseOptimizer(t, &opening{m: &fake{}}) }

func TestProse(t *testing.T) {
	in := strings.Join([]string{
		"# Title with drop words and more words here",
		"keep one dropb keep three dropd",
		"keep five 42 and `code` drops",
		"",
		"```",
		"drop inside code fence stays as is here",
		"```",
		"",
		"drop a b",
		"- drop list item stays as is as well",
	}, "\n")
	want := strings.Join([]string{
		"# Title with drop words and more words here",
		"keep one keep three",
		"keep five 42 and `code`",
		"",
		"```",
		"drop inside code fence stays as is here",
		"```",
		"",
		"drop a b",
		"- drop list item stays as is as well",
	}, "\n")
	o := working(t)
	defer o.Shutdown(t.Context())
	if got := strings.Join(o.prose(strings.Split(in, "\n")), "\n"); got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestOptimizeText(t *testing.T) {
	body := "keep one dropb keep three dropd"
	tests := []struct{ name, mode, want string }{
		{"compressed", "ok", "keep one keep three"},
		{"wrong flag count", "short", body},
		{"model fails to load", "broken", body},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &opening{m: &fake{}}
			switch tt.mode {
			case "short":
				op.m = &short{}
			case "broken":
				op.m, op.err = nil, errors.New("no model files")
			}
			o := proseOptimizer(t, op)
			defer o.Shutdown(t.Context())
			p := &payload.Payload{Blocks: []payload.Block{{Type: "doc/text", Text: body}}}
			if err := o.Optimize(t.Context(), p); err != nil {
				t.Fatal(err)
			}
			if got := strings.TrimSpace(p.Blocks[0].Text); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTabularAndMarkersUntouched(t *testing.T) {
	table := []string{"drop1  drop2  drop3", "drop4  drop5  drop6", "drop7  drop8  drop9", "dropa  dropb  dropc"}
	prose := "keep one dropx keep three dropy"
	marker := []string{"… 12 lines omitted", "[gascoigne: optimized 10→5 tokens]"}
	o := working(t)
	defer o.Shutdown(t.Context())
	in := append(append(append([]string{}, table...), "", prose, ""), marker...)
	want := append(append(append([]string{}, table...), "", "keep one keep three", ""), marker...)
	if got := o.prose(in); !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
}

func TestLoadFailureNamesInstall(t *testing.T) {
	o := proseOptimizer(t, &opening{err: errors.New("open tokenizer.json: no such file")})
	_, err := o.model.keep([]string{"a"})
	if err == nil || !strings.Contains(err.Error(), "gascoigne prose install") || !strings.Contains(err.Error(), "tokenizer.json") {
		t.Fatalf("got %v", err)
	}
}

func TestStartWarmsModel(t *testing.T) {
	for _, warm := range []bool{true, false} {
		op := &opening{m: &fake{}}
		o := proseOptimizer(t, op)
		o.cfg.Warm = warm
		if err := o.Start(t.Context(), nil); err != nil {
			t.Fatal(err)
		}
		for deadline := time.Now().Add(5 * time.Second); warm && op.calls.Load() == 0 && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
		}
		o.Shutdown(t.Context())
		if got := op.calls.Load() > 0; got != warm {
			t.Errorf("warm=%v: model opened = %v", warm, got)
		}
	}
}

func TestShutdownClosesModel(t *testing.T) {
	f := &fake{}
	o := proseOptimizer(t, &opening{m: f})
	o.model.keep([]string{"a"})
	o.Shutdown(t.Context())
	if !f.closed.Load() {
		t.Fatal("model still open")
	}
}

func TestGivenUp(t *testing.T) {
	op := &opening{err: errors.New("boom")}
	o := proseOptimizer(t, op)
	for range maxFailures {
		o.model.keep([]string{"a"})
	}
	if _, err := o.model.keep([]string{"a"}); err != errGivenUp {
		t.Fatalf("got %v, want errGivenUp", err)
	}
	if got := op.calls.Load(); got != maxFailures {
		t.Fatalf("opened %d times, want %d", got, maxFailures)
	}
}

type panicking struct{ fake }

func (*panicking) Keep([]string, float64) ([]bool, error) { panic("native fault") }

func TestPanicIsAFailure(t *testing.T) {
	o := proseOptimizer(t, &opening{m: &panicking{}})
	if _, err := o.model.keep([]string{"a"}); err == nil || !strings.Contains(err.Error(), "native fault") {
		t.Fatalf("got %v", err)
	}
}

func TestDisabledByDefault(t *testing.T) {
	o := &Optimizer{cfg: &Config{}}
	body := "drop one dropb dropc dropd\n\n\n\nx"
	p := &payload.Payload{Blocks: []payload.Block{{Type: "doc/text", Text: body}}}
	if err := o.Optimize(t.Context(), p); err != nil || p.Blocks[0].Text != body {
		t.Fatalf("text must stay byte-exact: %q, %v", p.Blocks[0].Text, err)
	}
}

func TestValidate(t *testing.T) {
	ok := Config{Enabled: true, Model: "default", Device: "cpu", Threshold: 0.5, MinWords: 1}
	bad := ok
	bad.Threshold = 1
	worse := ok
	worse.Device = "tpu"
	unknown := ok
	unknown.Model = "huge"
	if ok.Validate() != nil || bad.Validate() == nil || worse.Validate() == nil || unknown.Validate() == nil {
		t.Fatal("threshold must be inside (0, 1), device and model known ones")
	}
}

func TestIdleUnload(t *testing.T) {
	op := &opening{m: &fake{}}
	cfg := &Config{Enabled: true, Device: "auto", Threshold: 0.5, MinWords: 4, Idle: 50 * time.Millisecond}
	l := &loaded{cfg: cfg, open: op.open, log: slog.New(slog.DiscardHandler)}
	defer l.close()

	if _, err := l.keep([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for range 200 {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal(what)
	}
	m := op.m.(*fake)
	waitFor("idle model not unloaded", m.closed.Load)

	m.closed.Store(false)
	if _, err := l.keep([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if op.calls.Load() != 2 {
		t.Fatalf("model opened %d times, want a reload after the unload", op.calls.Load())
	}
	waitFor("reloaded model not unloaded again", m.closed.Load)
}
