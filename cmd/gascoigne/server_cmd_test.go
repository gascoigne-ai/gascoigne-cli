package main

import (
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func TestCmdServerSubcommands(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	addr := freeAddr(t)
	var err error
	out := capture(t, func() { err = cmdServer(t.Context(), []string{"status", "-addr", addr}) })
	if err != nil || !strings.Contains(out, "not running on "+addr) {
		t.Errorf("status of a stopped server: %q, %v", out, err)
	}
	out = capture(t, func() { err = cmdServer(t.Context(), []string{"stop", "-addr", addr}) })
	if err != nil || !strings.Contains(out, "not running on "+addr) {
		t.Errorf("stop of a stopped server: %q, %v", out, err)
	}
	if err := cmdServer(t.Context(), []string{"bogus"}); err == nil || !strings.Contains(err.Error(), `got "bogus"`) {
		t.Errorf("unknown subcommand: %v", err)
	}
}

func TestCmdServerRunStatusStop(t *testing.T) {
	sandbox(t)
	t.Setenv("GASCOIGNE_SERVER", "off")
	t.Setenv(ledger.EnvPath, filepath.Join(t.TempDir(), "l.db"))
	addr := freeAddr(t)
	done := make(chan error, 1)
	go func() { done <- cmdServer(t.Context(), []string{"run", "-addr", addr, "-idle", "0"}) }()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := health(addr); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not come up")
		}
		time.Sleep(20 * time.Millisecond)
	}
	var err error
	out := capture(t, func() { err = cmdServer(t.Context(), []string{"status", "-addr", addr}) })
	if err != nil || !strings.Contains(out, "running on http://"+addr) || !strings.Contains(out, "exits after never idle") {
		t.Errorf("status: %q, %v", out, err)
	}
	out = capture(t, func() { err = cmdServer(t.Context(), []string{"stop", "-addr", addr}) })
	if err != nil || !strings.Contains(out, "stopped (pid ") {
		t.Errorf("stop: %q, %v", out, err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run ended with %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("server did not exit after stop")
	}
}
