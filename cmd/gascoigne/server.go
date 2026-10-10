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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/lukaszb/gascoigne/extension/ledger"
)

const defaultServerAddr = "127.0.0.1:7878"

// serverInfo is what a running server says about itself.
type serverInfo struct {
	PID     int       `json:"pid"`
	Build   string    `json:"build"`
	Ledger  string    `json:"ledger"`
	Addr    string    `json:"addr"`
	Started time.Time `json:"started"`
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
	flags := func(fs *flag.FlagSet) {
		fs.StringVar(&addr, "addr", serverAddr(), "listen address, loopback only (default $GASCOIGNE_SERVER_ADDR or "+defaultServerAddr+")")
	}
	switch sub {
	case "run":
		return withLedger(ctx, "server run", args, 0, flags, func(l *ledger.Ledger, _ *common, _ []string) error {
			return runServer(ctx, l, addr)
		})
	case "start":
		return withLedger(ctx, "server start", args, 0, flags, func(l *ledger.Ledger, _ *common, _ []string) error {
			if err := ensureServer(ctx, addr, l.Path(), true); err != nil {
				return err
			}
			fmt.Printf("gascoigne dashboard: http://%s\n", addr)
			return nil
		})
	case "stop", "status":
		fs := flag.NewFlagSet("server "+sub, flag.ContinueOnError)
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
			fmt.Printf("gascoigne server: running on http://%s\n  pid %d, up %s\n  ledger %s\n  build %s\n",
				info.Addr, info.PID, time.Since(info.Started).Round(time.Second), info.Ledger, info.Build)
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

// runServer serves the dashboard in the foreground until ctx ends or a
// client asks it to stop.
func runServer(ctx context.Context, l *ledger.Ledger, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	info := serverInfo{PID: os.Getpid(), Build: build(), Ledger: l.Path(), Addr: ln.Addr().String(), Started: time.Now()}
	srv := &http.Server{Handler: dashboard(l, time.Second, info, stop), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.WithoutCancel(ctx))
	}()
	fmt.Fprintf(os.Stderr, "gascoigne server: http://%s (pid %d, ledger %s)\n", info.Addr, info.PID, info.Ledger)
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
	switch serverAction(info, err == nil, ledgerPath, build()) {
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
// address, for a command using ledgerPath from binary build.
func serverAction(info serverInfo, running bool, ledgerPath, build string) action {
	switch {
	case !running:
		return startServer
	case info.Ledger != ledgerPath: // someone else's ledger, e.g. an isolated eval
		return keepServer
	case info.Build != build:
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
	if err := os.MkdirAll(filepath.Dir(serverLog()), 0o755); err != nil {
		return err
	}
	log, err := os.OpenFile(serverLog(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, "server", "run", "-addr", addr)
	cmd.Stdout, cmd.Stderr = log, log
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
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
	exe, err := os.Executable()
	if err != nil {
		return "unknown"
	}
	st, err := os.Stat(exe)
	if err != nil {
		return "unknown"
	}
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%d|%d", exe, st.Size(), st.ModTime().UnixNano()))
	return hex.EncodeToString(sum[:6])
}
