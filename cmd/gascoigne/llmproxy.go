package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/httpproxyreceiver"
)

// The LLM API proxy is served by the background server under /v1/, so
// agents' base URL is the server's address: one process, one port.

// proxyOff reports GASCOIGNE_PROXY=off: agents are never pointed at it.
func proxyOff() bool { return os.Getenv("GASCOIGNE_PROXY") == "off" }

// proxyForced reports GASCOIGNE_PROXY=on: the proxy is used for Claude even
// with a claude.ai login, on the user's own judgment (see claudeProxyOK).
func proxyForced() bool { return os.Getenv("GASCOIGNE_PROXY") == "on" }

// claudeKey reports an Anthropic API key or gateway token in the
// environment, the credentials gateways are documented for.
func claudeKey() bool {
	return os.Getenv("ANTHROPIC_API_KEY") != "" || os.Getenv("ANTHROPIC_AUTH_TOKEN") != ""
}

// loginNote explains why Claude talks to Anthropic directly.
const loginNote = "LLM proxy not used for Claude: no ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN, so the credential is " +
	"a claude.ai login, whose terms are meant for ordinary use of Claude Code and do not cover tools that " +
	"intermediate its session token (docs/agents.md). Hooks and MCP stay on. GASCOIGNE_PROXY=on uses it anyway."

// claudeProxyOK reports whether Claude's API traffic may go through the
// proxy: with an API key or gateway token, or when the user forces it.
// A claude.ai login stays direct.
func claudeProxyOK() bool { return claudeKey() || proxyForced() }

var loginNoteOnce sync.Once

func proxyURL() string { return "http://" + serverAddr() }

// llmProxy builds the proxy from the user's config: its receiver settings
// and pipeline "proxy", run like the warm pipelines (one payload at a time,
// under mu). A non-empty upstream replaces every API's upstream (replay).
// close shuts the pipeline down.
func llmProxy(ctx context.Context, mu *sync.Mutex, ledgerPath, upstream string, log *slog.Logger) (*httpproxyreceiver.Receiver, func(), error) {
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
	if upstream != "" {
		rc.Anthropic, rc.OpenAI, rc.Gemini = upstream, upstream, upstream
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

// proxyForClaude is the variable pointing a wrapped Claude run at the
// proxy, when claudeProxyVar allows one.
func proxyForClaude() []string {
	if proxyOff() {
		return nil
	}
	key, path, note := claudeProxyVar()
	if key == "" {
		if note != "" {
			loginNoteOnce.Do(func() { fmt.Fprintln(os.Stderr, "gascoigne:", note) })
		}
		return nil
	}
	if u := runProxy(); u != "" {
		return []string{key + "=" + u + path}
	}
	return nil
}

// claudeProxyVar picks the base URL variable Claude Code reads for its
// API and the proxy path for it: Anthropic's API with an API key or gateway
// token, Amazon Bedrock with a Bedrock API key. "" with a note when the
// proxy must not be used: a claude.ai login (terms of use), Bedrock with
// AWS signatures (they sign the body the proxy would change), or a base
// URL the user set (their gateway stays).
func claudeProxyVar() (key, path, note string) {
	if truthy(os.Getenv("CLAUDE_CODE_USE_BEDROCK")) {
		region := cmp.Or(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"))
		switch {
		case userGateway("ANTHROPIC_BEDROCK_BASE_URL"):
			return "", "", ""
		case os.Getenv("AWS_BEARER_TOKEN_BEDROCK") == "":
			return "", "", bedrockNote
		case region == "":
			return "", "", "LLM proxy not used for Claude on Bedrock: set AWS_REGION."
		}
		return "ANTHROPIC_BEDROCK_BASE_URL", "/bedrock/" + region, ""
	}
	if truthy(os.Getenv("CLAUDE_CODE_USE_VERTEX")) || truthy(os.Getenv("CLAUDE_CODE_USE_FOUNDRY")) ||
		userGateway("ANTHROPIC_BASE_URL") {
		return "", "", ""
	}
	if !claudeProxyOK() {
		return "", "", loginNote
	}
	return "ANTHROPIC_BASE_URL", "", ""
}

// userGateway reports a base URL the user set to something other than
// gascoigne's own proxy (left by install -proxy).
func userGateway(key string) bool {
	u := os.Getenv(key)
	return u != "" && u != proxyURL() && !strings.HasPrefix(u, proxyURL()+"/")
}

// bedrockNote explains why Claude on Bedrock with AWS credentials stays direct.
const bedrockNote = "LLM proxy not used for Claude on Bedrock: AWS credentials sign each request body, which the " +
	"proxy would change. Use a Bedrock API key (AWS_BEARER_TOKEN_BEDROCK) to route it through gascoigne."

func truthy(v string) bool {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
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

// proxyForGemini points Gemini CLI's API client at the proxy when it uses an
// API key; a Google login talks to another backend, left direct.
func proxyForGemini() []string {
	if os.Getenv("GEMINI_API_KEY") == "" || os.Getenv("GOOGLE_GEMINI_BASE_URL") != "" {
		return nil
	}
	if u := runProxy(); u != "" {
		return []string{"GOOGLE_GEMINI_BASE_URL=" + u}
	}
	return nil
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
	// Only providers on an API key: subscription logins stay direct.
	base := map[string]any{"options": map[string]any{"baseURL": u + "/v1"}}
	out := map[string]any{}
	if os.Getenv("OPENAI_API_KEY") != "" {
		out["openai"] = base
	}
	if claudeProxyOK() {
		out["anthropic"] = base
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
