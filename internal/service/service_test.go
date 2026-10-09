package service

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/internal/registry"
	"github.com/lukaszb/gascoigne/payload"
)

type empty struct{}

type upper struct{ component.Nop }

func (upper) Optimize(_ context.Context, p *payload.Payload) error {
	for i := range p.Blocks {
		p.Blocks[i].Text = strings.ToUpper(p.Blocks[i].Text)
	}
	return nil
}

type boom struct{ component.Nop }

func (boom) Optimize(_ context.Context, p *payload.Payload) error {
	p.Blocks[0].Text = "partial"
	return errors.New("boom")
}

type capture struct {
	component.Nop
	got []*payload.Payload
}

func (c *capture) Export(_ context.Context, p *payload.Payload) error {
	c.got = append(c.got, p)
	return nil
}

func factory[C component.Component](kind component.Kind, typ string, c C) component.Factory {
	return component.NewFactory(kind, typ, func() *empty { return &empty{} },
		func(context.Context, component.Settings, *empty) (component.Component, error) { return c, nil })
}

const cfgYAML = `
extensions: { ledger: {} }
receivers: { stdin: {} }
filters: { bypass: {}, history: { max_failures: 1 } }
optimizers: { upper: {}, boom: {}, passthrough: {} }
exporters: { capture: {} }
router:
  routes:
    - { name: up, match: { attrs: { cmd: "up *" } }, optimizers: [upper] }
    - { name: fail, match: { attrs: { cmd: "boom*" } }, optimizers: [boom] }
  default: [passthrough]
pipelines:
  cli: { receivers: [stdin], filters: [bypass, history], exporters: [capture] }
`

func newService(t *testing.T, yaml string) (*Service, *capture) {
	t.Helper()
	t.Setenv(ledger.EnvPath, filepath.Join(t.TempDir(), "l.db"))
	cfg, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	exp := &capture{}
	factories := append(registry.Factories(),
		factory(component.KindOptimizer, "upper", upper{}),
		factory(component.KindOptimizer, "boom", boom{}),
		factory(component.KindExporter, "capture", exp),
	)
	s, err := New(t.Context(), cfg, factories, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()) })
	return s, exp
}

func process(t *testing.T, s *Service, cmd, text string, attrs ...string) *payload.Payload {
	t.Helper()
	p := payload.New(payload.CmdOutput, text)
	p.Set(payload.AttrCmd, cmd)
	for i := 0; i < len(attrs); i += 2 {
		p.Set(attrs[i], attrs[i+1])
	}
	pl, _ := s.Pipeline("cli")
	out, err := pl.Process(t.Context(), p)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestOptimizedIsReversible(t *testing.T) {
	s, exp := newService(t, cfgYAML)
	out := process(t, s, "up x", "hello\n")
	if out.Text() != "HELLO\n" || len(exp.got) != 1 {
		t.Fatalf("out = %q, exported %d", out.Text(), len(exp.got))
	}
	id := out.Attrs[payload.AttrRawID]
	key, raw, err := s.Ledger().GetRaw(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if key != "cmd:up x" || raw.Text() != "hello\n" {
		t.Fatalf("raw = %q %q", key, raw.Text())
	}
}

func TestUnchangedHasNoRawID(t *testing.T) {
	s, _ := newService(t, cfgYAML)
	if out := process(t, s, "ls", "a"); out.Attrs[payload.AttrRawID] != "" {
		t.Fatalf("raw_id set for unchanged payload: %v", out.Attrs)
	}
}

func TestBypassIsVerbatim(t *testing.T) {
	s, _ := newService(t, cfgYAML)
	out := process(t, s, "up x", "hello", payload.AttrRaw, "true")
	if out.Text() != "hello" || out.Attrs[payload.AttrRawID] != "" {
		t.Fatalf("out = %+v", out)
	}
}

func TestOptimizerErrorFallsBackAndIsLearned(t *testing.T) {
	s, _ := newService(t, cfgYAML)
	if out := process(t, s, "boom", "orig"); out.Text() != "orig" {
		t.Fatalf("out = %q, want original", out.Text())
	}
	if n, _ := s.Ledger().Failures(t.Context(), "cmd:boom"); n != 1 {
		t.Fatalf("failures = %d, want 1", n)
	}
	// Second run is bypassed by history before reaching the optimizer.
	if err := s.Ledger().MarkFailure(t.Context(), "cmd:up x", component.ReasonRawRequested); err != nil {
		t.Fatal(err)
	}
	if out := process(t, s, "up x", "hello"); out.Text() != "hello" {
		t.Fatalf("known-bad key optimized: %q", out.Text())
	}
}

func TestOptionsAttrsDoNotOverride(t *testing.T) {
	s, _ := newService(t, cfgYAML)
	s.attrs = map[string]string{payload.AttrCmd: "up from-cli", "extra": "1"}
	out := process(t, s, "ls", "a")
	if out.Attrs[payload.AttrCmd] != "ls" || out.Attrs["extra"] != "1" {
		t.Fatalf("attrs = %v", out.Attrs)
	}
}

func TestOptimizersRequireLedger(t *testing.T) {
	yaml := strings.Replace(cfgYAML, "extensions: { ledger: {} }", "", 1)
	yaml = strings.Replace(yaml, "history: { max_failures: 1 }", "", 1)
	yaml = strings.Replace(yaml, "[bypass, history]", "[bypass]", 1)
	cfg, err := config.Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	factories := append(registry.Factories(),
		factory(component.KindOptimizer, "upper", upper{}),
		factory(component.KindOptimizer, "boom", boom{}),
		factory(component.KindExporter, "capture", &capture{}))
	if _, err := New(t.Context(), cfg, factories, Options{}); err == nil || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("err = %v, want ledger requirement", err)
	}
}

func TestUnknownTypeAndBadConfig(t *testing.T) {
	for _, tt := range []struct{ from, to, want string }{
		{"capture: {}", "nope: {}", `unknown type "nope"`},
		{"history: { max_failures: 1 }", "history: { max_failures: 0 }", "max_failures"},
		{"history: { max_failures: 1 }", "history: { bogus: 1 }", "field bogus"},
	} {
		yaml := strings.Replace(cfgYAML, tt.from, tt.to, 1)
		yaml = strings.Replace(yaml, "exporters: [capture]", "exporters: []", 1)
		cfg, err := config.Parse([]byte(yaml))
		if err != nil {
			t.Fatal(err)
		}
		factories := append(registry.Factories(),
			factory(component.KindOptimizer, "upper", upper{}),
			factory(component.KindOptimizer, "boom", boom{}))
		if _, err := New(t.Context(), cfg, factories, Options{}); err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s: err = %v, want %q", tt.to, err, tt.want)
		}
	}
}

type recorder struct {
	name string
	log  *[]string
}

func (r recorder) Start(context.Context, component.Host) error {
	*r.log = append(*r.log, "start "+r.name)
	return nil
}
func (r recorder) Shutdown(context.Context) error {
	*r.log = append(*r.log, "stop "+r.name)
	return nil
}
func (recorder) Run(context.Context) error { return nil }
func (recorder) Filter(context.Context, *payload.Payload) (component.Decision, error) {
	return component.Continue, nil
}
func (recorder) Optimize(context.Context, *payload.Payload) error { return nil }
func (recorder) Export(context.Context, *payload.Payload) error   { return nil }

func TestLifecycleOrder(t *testing.T) {
	var log []string
	rec := func(kind component.Kind) component.Factory {
		return factory(kind, "r", recorder{string(kind), &log})
	}
	cfg, err := config.Parse([]byte(`
extensions: { r: {} }
receivers: { r: {} }
filters: { r: {} }
optimizers: { r: {} }
exporters: { r: {} }
pipelines: { p: { receivers: [r], filters: [r], exporters: [r] } }
`))
	if err != nil {
		t.Fatal(err)
	}
	factories := []component.Factory{
		rec(component.KindExtension), rec(component.KindReceiver), rec(component.KindFilter),
		rec(component.KindOptimizer), rec(component.KindExporter),
	}
	s, err := New(t.Context(), cfg, factories, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := s.Shutdown(t.Context()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"start extension", "start exporter", "start optimizer", "start filter", "start receiver",
		"stop receiver", "stop filter", "stop optimizer", "stop exporter", "stop extension",
	}
	if !slices.Equal(log, want) {
		t.Fatalf("order = %v\nwant    %v", log, want)
	}
}
