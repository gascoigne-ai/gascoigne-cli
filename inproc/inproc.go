// Package inproc runs the gascoigne optimization pipeline inside the calling process. It is the
// public face of the engine for libraries that cannot import internal packages, such as the C ABI
// shared library of gascoigne-sdk. The configuration is owned here: built-in defaults, the user's
// file, then whatever the host passes in, merged in that order.
package inproc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/gascoigne-ai/gascoigne-cli/configs"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
	"github.com/gascoigne-ai/gascoigne-cli/internal/service"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Pipeline is the chain SDK payloads run through: the same filters and optimizers as
// `gascoigne run`.
const Pipeline = "cli"

// hiddenPrefix marks ledger keys of instruction text, which is never handed back to SDKs.
const hiddenPrefix = "prompt:"

// ErrNotFound means the ledger has no original under the id.
var ErrNotFound = errors.New("original not found (expired or unknown id)")

// Options choose the configuration of an Engine. The zero value means defaults plus the user's
// config file.
type Options struct {
	// Config is YAML laid over the defaults and the user's file: maps merge key by key, lists
	// and values replace.
	Config string
	// Set holds "dotted.path=value" assignments applied after Config.
	Set []string
	// Only keeps just these optimizers in every chain.
	Only []string
	// Disable drops these optimizers from every chain.
	Disable []string
	// IgnoreUserFile skips the user's config file ($GASCOIGNE_CONFIG or the per-user config dir).
	IgnoreUserFile bool
}

// Request is one tool result to optimize; empty attributes are left out.
type Request struct {
	Text    string
	Tool    string
	Cmd     string
	URL     string
	Query   string
	Session string
	Agent   string
	Model   string
}

// Result is what the model should read, with token counts and the id of the original.
type Result struct {
	Text      string
	RawID     string
	RawTokens int
	OutTokens int
	Skipped   string
}

// Engine is a started pipeline service. It is safe for concurrent use.
type Engine struct {
	mu  sync.Mutex
	svc *service.Service
	pl  *service.Pipeline
	led *ledger.Ledger
}

// ConfigYAML returns the merged configuration the options describe, as the engine would run it.
func ConfigYAML(o Options) ([]byte, error) {
	_, data, err := resolve(o)
	return data, err
}

// New resolves the configuration, then starts the filters, optimizers and ledger.
func New(ctx context.Context, o Options) (*Engine, error) {
	cfg, _, err := resolve(o)
	if err != nil {
		return nil, err
	}
	// Only the optimize chain runs here: nothing listens and nothing exports.
	for name, pc := range cfg.Pipelines {
		pc.Receivers, pc.Exporters = nil, nil
		cfg.Pipelines[name] = pc
	}
	cfg.Receivers, cfg.Exporters = nil, nil
	svc, err := service.New(ctx, cfg, registry.Factories(), service.Options{})
	if err != nil {
		return nil, err
	}
	if err := svc.Start(ctx); err != nil {
		return nil, err
	}
	pl, ok := svc.Pipeline(Pipeline)
	if !ok {
		_ = svc.Shutdown(context.WithoutCancel(ctx))
		return nil, fmt.Errorf("config has no pipeline %q", Pipeline)
	}
	led, _ := svc.Ledger().(*ledger.Ledger)
	return &Engine{svc: svc, pl: pl, led: led}, nil
}

// Optimize runs one tool result through the pipeline.
func (e *Engine) Optimize(ctx context.Context, r Request) (Result, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	p := payload.New(payload.ToolResult, r.Text)
	for k, v := range map[string]string{
		payload.AttrTool: r.Tool, payload.AttrCmd: r.Cmd, payload.AttrURL: r.URL,
		payload.AttrQuery: r.Query, payload.AttrSession: r.Session,
		payload.AttrAgent: r.Agent, payload.AttrModel: r.Model,
	} {
		if v != "" {
			p.Attrs[k] = v
		}
	}
	out, err := e.pl.Optimize(ctx, p)
	if err != nil {
		return Result{}, err
	}
	res := Result{
		Text:    out.Text(),
		RawID:   out.Attrs[payload.AttrRawID],
		Skipped: out.Attrs[payload.AttrSkip],
	}
	res.RawTokens, _ = strconv.Atoi(out.Attrs[payload.AttrRawTokens])
	res.OutTokens, _ = strconv.Atoi(out.Attrs[payload.AttrOutTokens])
	return res, nil
}

// Raw returns the original text behind a RawID.
func (e *Engine) Raw(ctx context.Context, id string) (string, error) {
	if e.led == nil {
		return "", ErrNotFound
	}
	key, p, err := e.led.GetRaw(ctx, id)
	if err != nil || strings.HasPrefix(key, hiddenPrefix) {
		return "", ErrNotFound
	}
	return p.Text(), nil
}

// Close stops the service and its ledger.
func (e *Engine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.svc.Shutdown(ctx)
}

// resolve merges the layers and parses the result.
func resolve(o Options) (*config.Config, []byte, error) {
	data := configs.Default
	if !o.IgnoreUserFile {
		var err error
		if data, err = withUserFile(data); err != nil {
			return nil, nil, err
		}
	}
	if strings.TrimSpace(o.Config) != "" {
		var err error
		if data, err = configs.Overlay(data, []byte(o.Config)); err != nil {
			return nil, nil, fmt.Errorf("config: %w", err)
		}
	}
	cfg, err := config.ParseWith(data, config.Overrides{Set: o.Set, Only: o.Only, Disable: o.Disable})
	if err != nil {
		return nil, nil, err
	}
	return cfg, data, nil
}

// withUserFile lays the user's file over base. A missing file is not an error, and the library
// never writes one: creating the template is the CLI's job.
func withUserFile(base []byte) ([]byte, error) {
	path := os.Getenv("GASCOIGNE_CONFIG")
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return base, nil
		}
		path = filepath.Join(dir, "gascoigne", "config.yaml")
	}
	over, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return base, nil
	}
	if err != nil {
		return nil, err
	}
	if bad := configs.Unknown(over); len(bad) > 0 {
		return nil, fmt.Errorf("config %s: not settings in this version: %s; "+
			"gascoigne config -upgrade drops them", path, strings.Join(bad, ", "))
	}
	merged, err := configs.Overlay(base, over)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	return merged, nil
}
