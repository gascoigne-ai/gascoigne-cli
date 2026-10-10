package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/lukaszb/gascoigne/receiver/httpproxyreceiver"
)

// The LLM API proxy runs as its own detached process (gascoigne proxy), so
// agent API traffic does not depend on the dashboard server, which restarts
// itself on new builds. Agents reach it through their base URL.

func proxyAddr() string {
	return cmp.Or(os.Getenv("GASCOIGNE_PROXY_ADDR"), httpproxyreceiver.DefaultListen)
}

func proxyURL() string { return "http://" + proxyAddr() }

// proxyOff reports GASCOIGNE_PROXY=off: agents are never pointed at it.
func proxyOff() bool { return os.Getenv("GASCOIGNE_PROXY") == "off" }

// cmdProxy runs the proxy in the foreground.
func cmdProxy(ctx context.Context, args []string) error {
	var c common
	fs := newFlags("proxy")
	c.register(fs)
	addr := fs.String("addr", proxyAddr(), "listen `address`, loopback (default $GASCOIGNE_PROXY_ADDR or "+httpproxyreceiver.DefaultListen+")")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c.sets = append(c.sets, "receivers.httpproxy.listen="+*addr)
	c.server, c.pipeline = true, "proxy"
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gascoigne proxy: http://%s (ANTHROPIC_BASE_URL=http://%s)\n", *addr, *addr)
	return errors.Join(svc.Run(ctx, "proxy"), svc.Shutdown(context.WithoutCancel(ctx)))
}

var proxyClient = &http.Client{Timeout: 300 * time.Millisecond}

func proxyHealth(addr string) (httpproxyreceiver.Health, error) {
	var h httpproxyreceiver.Health
	resp, err := proxyClient.Get("http://" + addr + httpproxyreceiver.HealthPath)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return h, fmt.Errorf("proxy health: %s", resp.Status)
	}
	return h, json.NewDecoder(resp.Body).Decode(&h)
}

// ensureProxy starts the proxy unless one answers. A proxy from an older
// binary is replaced (requests in flight may fail once and be retried by
// the agent); a newer one stays.
func ensureProxy(ctx context.Context, addr string) error {
	h, err := proxyHealth(addr)
	if err == nil {
		exe, _ := os.Executable()
		st, statErr := os.Stat(exe)
		if statErr != nil || h.Exe == exe && h.ExeSize == st.Size() && h.ExeTime == st.ModTime().UnixNano() ||
			st.ModTime().UnixNano() <= h.ExeTime {
			return nil
		}
		if err := terminate(h.PID); err != nil {
			return nil // cannot replace it here: keep using it
		}
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
			if _, err := proxyHealth(addr); err != nil {
				break
			}
		}
	}
	if err := spawnProxy(addr); err != nil {
		return err
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err := proxyHealth(addr); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("proxy did not start on %s; see %s", addr, proxyLog())
}

func spawnProxy(addr string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(proxyLog()), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(proxyLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, "proxy", "-addr", addr)
	cmd.Env = serverEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func proxyLog() string { return filepath.Join(filepath.Dir(serverLog()), "proxy.log") }

var proxyOnce = sync.OnceValue(func() string {
	if proxyOff() {
		return ""
	}
	if err := ensureProxy(context.Background(), proxyAddr()); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: LLM proxy not used:", err)
		return ""
	}
	return proxyURL()
})

// runProxy is the proxy's base URL for an agent started now, starting the
// proxy if needed; "" when it is off or could not start (the agent then
// talks to its API directly, as before).
func runProxy() string { return proxyOnce() }

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

// ensureProxyFor starts the proxy when this process's agent was pointed at
// it for good (install -proxy): Claude runs its SessionStart hook with the
// settings' env.
func ensureProxyFor() {
	if os.Getenv("ANTHROPIC_BASE_URL") == proxyURL() {
		if err := ensureProxy(context.Background(), proxyAddr()); err != nil {
			fmt.Fprintln(os.Stderr, "gascoigne hook:", err)
		}
	}
}
