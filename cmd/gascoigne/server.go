package main

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

const defaultServerAddr = "127.0.0.1:7878"

// serverInfo is what a running server says about itself.
type serverInfo struct {
	PID     int    `json:"pid"`
	Version string `json:"version"` // release and commit, for people; Build decides replacement
	Build   string `json:"build"`
	// Modified is when the binary was built or installed: an older one never
	// replaces a newer server.
	Modified time.Time `json:"modified"`
	Ledger   string    `json:"ledger"`
	Addr     string    `json:"addr"`
	Started  time.Time `json:"started"`
	Idle     string    `json:"idle,omitempty"` // exits after this long without a request; "" never
}

// serverAddr is where the one server per user listens.
func serverAddr() string { return cmp.Or(os.Getenv("GASCOIGNE_SERVER_ADDR"), defaultServerAddr) }

// serverOff reports GASCOIGNE_SERVER=off: commands then never start one.
func serverOff() bool { return os.Getenv("GASCOIGNE_SERVER") == "off" }

// cmdServer manages the background server hosting the dashboard. Commands
// that optimize start it when it is not running, so the dashboard is there
// whenever gascoigne is; optimization itself stays in each process.
func cmdServer(ctx context.Context, args []string) error {
	sub := "status"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	var addr string
	var idle time.Duration
	var open bool
	flags := func(fs *flag.FlagSet) {
		fs.StringVar(&addr, "addr", serverAddr(), "listen `address`, loopback only (default $GASCOIGNE_SERVER_ADDR or "+defaultServerAddr+")")
		if sub == "run" {
			fs.DurationVar(&idle, "idle", serverIdle(), "exit after this long without a request, 0 never (default $GASCOIGNE_SERVER_IDLE or 30m)")
		}
		if sub == "start" {
			fs.BoolVar(&open, "open", false, "open the dashboard in a browser")
		}
	}
	switch sub {
	case "run":
		err := withLedger(ctx, "server run", args, 0, flags, func(l *ledger.Ledger, _ *common, _ []string) error {
			return runServer(ctx, l, addr, idle)
		})
		if errors.Is(err, errNewBuild) {
			// The ledger is closed and the port free: the new binary takes over.
			fmt.Fprintln(os.Stderr, "gascoigne server: binary changed, restarting")
			return spawnServer(addr)
		}
		return err
	case "start":
		return withLedger(ctx, "server start", args, 0, flags, func(l *ledger.Ledger, _ *common, _ []string) error {
			if err := ensureServer(ctx, addr, l.Path(), true); err != nil {
				return err
			}
			fmt.Printf("gascoigne dashboard: http://%s\n", addr)
			if open {
				openBrowser("http://" + addr)
			}
			return nil
		})
	case "stop", "status":
		fs := newFlags("server " + sub)
		flags(fs)
		if err := fs.Parse(args); err != nil {
			return err
		}
		info, err := health(addr)
		if err != nil {
			fmt.Printf("gascoigne server: not running on %s\n", addr)
			return nil
		}
		if sub == "status" {
			fmt.Printf("gascoigne server: running on http://%s\n  version %s, pid %d, up %s, exits after %s idle\n  ledger %s\n  build %s\n",
				info.Addr, cmp.Or(info.Version, "unknown"), info.PID, time.Since(info.Started).Round(time.Second), cmp.Or(info.Idle, "never"), info.Ledger, info.Build)
			return nil
		}
		if err := stopServer(addr); err != nil {
			return err
		}
		fmt.Printf("gascoigne server: stopped (pid %d)\n", info.PID)
		return nil
	default:
		return fmt.Errorf("server: want run, start, stop or status, got %q", sub)
	}
}

// errNewBuild: the server's binary was replaced (rebuilt, upgraded).
var errNewBuild = errors.New("binary changed")

// runServer serves the dashboard in the foreground until ctx ends or a
// client asks it to stop. It returns errNewBuild once its binary changes,
// so the new version restarts it.
func runServer(ctx context.Context, l *ledger.Ledger, addr string, idle time.Duration) error {
	if !loopbackHost(addr) {
		return fmt.Errorf("server: %q is not a loopback address; the API has no login", addr)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	b, modified := binary()
	info := serverInfo{PID: os.Getpid(), Version: version.String(), Build: b, Modified: modified, Ledger: l.Path(), Addr: ln.Addr().String(), Started: time.Now()}
	if idle > 0 {
		info.Idle = idle.String()
	}
	w := newWarm(ctx, info.Build, info.Ledger)
	defer w.Close()
	mux := http.NewServeMux()
	mux.Handle("/", dashboard(l, time.Second, info, stop))
	mux.Handle("POST /api/optimize", localOnly(w))
	mux.Handle("POST /sdk/v1/optimize", localOnly(sdkOptimize(w)))
	mux.Handle("GET /sdk/v1/raw/{id}", localOnly(sdkRaw(l)))
	proxyOn := false
	proxy, closeProxy, err := llmProxy(ctx, &w.mu, info.Ledger, "", slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne server: LLM proxy off:", err)
	} else {
		defer closeProxy()
		proxyOn = true
		mux.Handle("/v1/", localOnly(proxy))
		mux.Handle("/v1beta/", localOnly(proxy))  // Gemini
		mux.Handle("/bedrock/", localOnly(proxy)) // Claude on Amazon Bedrock
	}
	mux.Handle("GET /sdk/v1/health", localOnly(sdkHealthHandler(info.Version, proxyOn)))
	var rebuilt atomic.Bool
	go watchBinary(ctx, b, 2*time.Second, func() string { b, _ := binary(); return b }, func() {
		rebuilt.Store(true)
		waitIdle(ctx, proxy) // agents' streams finish first
		stop()
	})
	act := newActivity(time.Now())
	go watchIdle(ctx, act, idle, min(time.Minute, max(idle/10, time.Second)), func() {
		fmt.Fprintf(os.Stderr, "gascoigne server: idle for %s, exiting\n", idle)
		stop()
	})
	srv := &http.Server{Handler: act.track(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 2 * time.Minute}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.WithoutCancel(ctx))
	}()
	fmt.Fprintf(os.Stderr, "gascoigne server: http://%s (pid %d, ledger %s)\n", info.Addr, info.PID, info.Ledger)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if rebuilt.Load() {
		return errNewBuild
	}
	return nil
}

// watchBinary calls changed once read reports a build other than current
// twice in a row, so a binary still being written is not started.
func watchBinary(ctx context.Context, current string, every time.Duration, read func() string, changed func()) {
	t := time.NewTicker(every)
	defer t.Stop()
	seen := current
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		b := read()
		switch {
		case b == current || b == "unknown":
			seen = current
		case b == seen:
			changed()
			return
		default:
			seen = b
		}
	}
}

// ensureServer starts the server unless one for this ledger and build
// already answers. A server from another build (gascoigne was rebuilt or
// upgraded) is replaced; one serving another ledger is left alone. wait
// blocks until the new server answers.
func ensureServer(ctx context.Context, addr, ledgerPath string, wait bool) error {
	if serverOff() {
		return nil
	}
	info, err := health(addr)
	b, modified := binary()
	switch serverAction(info, err == nil, ledgerPath, b, modified) {
	case keepServer:
		return nil
	case replaceServer:
		if err := stopServer(addr); err != nil {
			return err
		}
	}
	if err := spawnServer(addr); err != nil {
		return err
	}
	if !wait {
		return nil
	}
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		if _, err := health(addr); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
	return fmt.Errorf("server did not start on %s; see %s", addr, serverLog())
}

type action int

const (
	keepServer action = iota
	startServer
	replaceServer
)

// serverAction decides what to do about the server found (or not) on the
// address, for a command using ledgerPath from binary build, modified then.
// A server from an older binary is replaced; a newer one stays (this run
// optimizes locally), so two installed versions don't keep replacing each
// other.
func serverAction(info serverInfo, running bool, ledgerPath, build string, modified time.Time) action {
	switch {
	case !running:
		return startServer
	case info.Ledger != ledgerPath: // someone else's ledger, e.g. an isolated eval
		return keepServer
	case info.Build != build && (info.Modified.IsZero() || modified.After(info.Modified)):
		return replaceServer
	}
	return keepServer
}

// spawnServer starts "gascoigne server run" detached from this process, so
// it outlives the command (or agent) that started it.
func spawnServer(addr string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(serverLog()), 0o700); err != nil {
		return err
	}
	log, err := os.OpenFile(serverLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, "server", "run", "-addr", addr)
	cmd.Env = serverEnv(os.Environ())
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// serverEnv is the environment of a spawned server: GASCOIGNE_* variables
// of the command that spawns it (raw mode, overrides, agent identity) would
// otherwise apply to every later run it optimizes. Only those naming its
// ledger, config and address stay, and the release notice settings, which
// are the server's own (the opt-out must reach the process that makes the request).
func serverEnv(env []string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool {
		k, _, _ := strings.Cut(kv, "=")
		return strings.HasPrefix(k, "GASCOIGNE_") &&
			!slices.Contains([]string{ledger.EnvPath, "GASCOIGNE_CONFIG", "GASCOIGNE_SERVER_ADDR", "GASCOIGNE_SERVER_IDLE", "GASCOIGNE_RELEASE_CHECK", "GASCOIGNE_RELEASE_URL"}, k)
	})
}

func serverLog() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "gascoigne", "server.log")
}

var client = &http.Client{Timeout: 300 * time.Millisecond}

func health(addr string) (serverInfo, error) {
	var info serverInfo
	resp, err := client.Get("http://" + addr + "/api/health")
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("health: %s", resp.Status)
	}
	return info, json.NewDecoder(resp.Body).Decode(&info)
}

// stopServer asks the server to exit and waits until it no longer answers.
func stopServer(addr string) error {
	resp, err := client.Post("http://"+addr+"/api/shutdown", "application/json", bytes.NewReader(nil))
	if err != nil {
		return err
	}
	resp.Body.Close()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(30 * time.Millisecond) {
		if _, err := health(addr); err != nil {
			return nil
		}
	}
	return fmt.Errorf("server on %s did not stop", addr)
}

// build identifies this binary, so a rebuilt gascoigne replaces a server
// still running the old one.
func build() string {
	b, _ := binary()
	return b
}

// binary is this binary's build id (path, size, mtime) and mtime.
func binary() (string, time.Time) {
	exe, err := os.Executable()
	if err != nil {
		return "unknown", time.Time{}
	}
	st, err := os.Stat(exe)
	if err != nil {
		return "unknown", time.Time{}
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d", exe, st.Size(), st.ModTime().UnixNano()))
	return hex.EncodeToString(sum[:6]), st.ModTime()
}
