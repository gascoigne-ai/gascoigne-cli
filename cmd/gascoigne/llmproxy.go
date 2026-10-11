package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/receiver/httpproxyreceiver"
)

// The LLM API proxy is served by the background server under /v1/, so
// agents' base URL is the server's address: one process, one port.

// proxyOff reports GASCOIGNE_PROXY=off: agents are never pointed at it.
func proxyOff() bool { return os.Getenv("GASCOIGNE_PROXY") == "off" }

func proxyURL() string { return "http://" + serverAddr() }

// llmProxy builds the proxy from the user's config: its receiver settings
// and pipeline "proxy", run like the warm pipelines (one payload at a time,
// under mu). close shuts the pipeline down.
func llmProxy(ctx context.Context, mu *sync.Mutex, ledgerPath string, log *slog.Logger) (*httpproxyreceiver.Receiver, func(), error) {
	data, err := configData("")
	if err != nil {
		return nil, nil, err
	}
	cfg, err := config.ParseWith(data, config.Overrides{})
	if err != nil {
		return nil, nil, err
	}
	rc, _ := httpproxyreceiver.NewFactory().DefaultConfig().(*httpproxyreceiver.Config)
	if err := config.DecodeNode(cfg.Receivers["httpproxy"], rc); err != nil {
		return nil, nil, fmt.Errorf("receivers.httpproxy: %w", err)
	}
	if err := rc.Validate(); err != nil {
		return nil, nil, fmt.Errorf("receivers.httpproxy: %w", err)
	}
	svc, err := optimizerService(ctx, cfg, ledgerPath)
	if err != nil {
		return nil, nil, err
	}
	pl, ok := svc.Pipeline("proxy")
	if !ok {
		svc.Shutdown(ctx)
		return nil, nil, fmt.Errorf("no pipeline proxy in the config")
	}
	closer := func() { svc.Shutdown(context.WithoutCancel(ctx)) }
	r := httpproxyreceiver.New(rc, locked{mu, pl}, log)
	if err := r.Start(ctx, svc); err != nil {
		closer()
		return nil, nil, err
	}
	return r, closer, nil
}

// locked runs a pipeline one payload at a time: components need not be
// safe for concurrent use.
type locked struct {
	mu *sync.Mutex
	pl component.Pipeline
}

func (l locked) Process(ctx context.Context, p *payload.Payload) (*payload.Payload, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.pl.Process(ctx, p)
}

// waitIdle blocks until no proxied request is in flight (or ctx ends, or
// ten minutes pass), so a restart never cuts an agent's stream.
func waitIdle(ctx context.Context, p *httpproxyreceiver.Receiver) {
	if p == nil {
		return
	}
	for deadline := time.Now().Add(10 * time.Minute); p.Active() > 0 && time.Now().Before(deadline); {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

var proxyOnce = sync.OnceValue(func() string {
	if proxyOff() || serverOff() {
		return ""
	}
	if err := serverUp(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: LLM proxy not used:", err)
		return ""
	}
	return proxyURL()
})

// runProxy is the proxy's base URL for an agent started now, starting the
// background server if needed; "" when it is off or could not start (the
// agent then talks to its API directly, as before).
func runProxy() string { return proxyOnce() }

// serverUp makes sure the background server answers, as gascoigne
// server start does.
func serverUp() error {
	if _, err := health(serverAddr()); err == nil {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	out, err := exec.Command(exe, "server", "start").CombinedOutput()
	if err != nil {
		return fmt.Errorf("server start: %v: %s", err, out)
	}
	return nil
}

// proxyForClaude is ANTHROPIC_BASE_URL for a wrapped run, unless the user
// already points Claude Code at a gateway of their own.
func proxyForClaude() []string {
	if os.Getenv("ANTHROPIC_BASE_URL") != "" {
		return nil
	}
	if u := runProxy(); u != "" {
		return []string{"ANTHROPIC_BASE_URL=" + u}
	}
	return nil
}

// proxyForCodex adds a model provider through the proxy when codex uses an
// API key; a ChatGPT login talks to another backend, left direct.
func proxyForCodex() []string {
	if os.Getenv("OPENAI_API_KEY") == "" {
		return nil
	}
	u := runProxy()
	if u == "" {
		return nil
	}
	return []string{
		"-c", `model_providers.gascoigne={name="gascoigne",base_url="` + u + `/v1",env_key="OPENAI_API_KEY",wire_api="responses"}`,
		"-c", `model_provider="gascoigne"`,
	}
}

// proxyForOpencode points opencode's Anthropic and OpenAI providers at the
// proxy (merged over the user's config).
func proxyForOpencode() map[string]any {
	if os.Getenv("ANTHROPIC_BASE_URL") != "" {
		return nil
	}
	u := runProxy()
	if u == "" {
		return nil
	}
	base := map[string]any{"options": map[string]any{"baseURL": u + "/v1"}}
	return map[string]any{"anthropic": base, "openai": base}
}

// ensureProxyFor starts the server when this process's agent was pointed
// at it for good (install -proxy): Claude runs its SessionStart hook with
// the settings' env.
func ensureProxyFor() {
	if os.Getenv("ANTHROPIC_BASE_URL") == proxyURL() && !serverOff() {
		if err := serverUp(); err != nil {
			fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
		}
	}
}
