package externaloptimizer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// keepWarn is what the helper child does to a log: keep warnings and errors,
// say how many lines went. Real-shaped input comes from the eval fixtures.
func keepWarn(text string) string {
	var kept []string
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for _, l := range lines {
		if strings.Contains(l, " WARN") || strings.Contains(l, " ERROR") {
			kept = append(kept, l)
		}
	}
	return fmt.Sprintf("[%d of %d lines]\n%s\n", len(kept), len(lines), strings.Join(kept, "\n"))
}

// TestHelperProcess is the child other tests start (the test binary itself),
// steered by EXTOPT_MODE. It is not a test when run normally.
func TestHelperProcess(t *testing.T) {
	mode := os.Getenv("EXTOPT_MODE")
	if mode == "" {
		t.Skip("helper child")
	}
	switch mode {
	case "text", "text-stderr", "text-env":
		fmt.Fprint(os.Stderr, "noise on stderr\n")
		in, _ := io.ReadAll(os.Stdin)
		if mode == "text-env" {
			fmt.Print(strings.Join(os.Environ(), "\n"))
			break
		}
		fmt.Print(keepWarn(string(in)))
	case "text-attr":
		fmt.Printf("%s|%s|%s", os.Getenv("GASCOIGNE_BLOCK_TYPE"), os.Getenv("GASCOIGNE_ATTR_CMD"), os.Getenv("GASCOIGNE_PROTOCOL"))
	case "exit1":
		fmt.Fprint(os.Stderr, "boom")
		os.Exit(1)
	case "sleep":
		time.Sleep(time.Minute)
	case "empty":
	case "grow":
		in, _ := io.ReadAll(os.Stdin)
		fmt.Print(string(in) + string(in))
	case "json":
		var req Request
		json.NewDecoder(os.Stdin).Decode(&req)
		out := keepWarn(req.Block.Text)
		json.NewEncoder(os.Stdout).Encode(Response{Version: Version, Text: &out})
	case "json-bad":
		fmt.Print("not json")
	case "json-error":
		json.NewEncoder(os.Stdout).Encode(Response{Version: Version, Error: "cannot parse"})
	case "json-version":
		out := "x"
		json.NewEncoder(os.Stdout).Encode(Response{Version: 99, Text: &out})
	case "json-skip":
		var req Request
		json.NewDecoder(os.Stdin).Decode(&req)
		json.NewEncoder(os.Stdout).Encode(Response{Version: Version, Text: &req.Block.Text, SkipReason: "not a log"})
	case "rpc", "rpc-crash-once":
		rpcChild(mode == "rpc-crash-once")
	}
	os.Exit(0)
}

func rpcChild(crashOnce bool) {
	marker := os.Getenv("EXTOPT_MARKER")
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(nil, 16<<20)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		var req struct {
			Hello int `json:"hello"`
			Request
		}
		json.Unmarshal(sc.Bytes(), &req)
		if req.Hello != 0 {
			enc.Encode(Response{Version: Version})
			continue
		}
		if _, err := os.Stat(marker); crashOnce && err != nil {
			os.WriteFile(marker, nil, 0o600)
			os.Exit(3)
		}
		out := keepWarn(req.Block.Text)
		enc.Encode(Response{Version: Version, ID: req.ID, Text: &out})
	}
}

func helperConfig(mode string, protocol string) *Config {
	return &Config{
		Command:        []string{os.Args[0], "-test.run=^TestHelperProcess$"},
		Protocol:       protocol,
		Timeout:        10 * time.Second,
		MaxOutputBytes: 8 << 20,
		Env:            map[string]string{"EXTOPT_MODE": mode},
	}
}

func newOpt(t *testing.T, cfg *Config) *Optimizer {
	t.Helper()
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	o := &Optimizer{cfg: cfg, id: component.ID{Type: typ, Name: "t"}, codec: codecs[cfg.Protocol](cfg, log)}
	t.Cleanup(func() { o.Shutdown(context.Background()) })
	return o
}

func appLog(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../../testdata/eval/log/app.log")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func logPayload(t *testing.T) *payload.Payload {
	p := payload.New(payload.CmdOutput, appLog(t))
	p.Blocks[0].Type = "log/app"
	p.Set(payload.AttrCmd, "tail -n 2000 app.log")
	return p
}

func TestOptimize(t *testing.T) {
	tests := []struct {
		name, mode, protocol string
		wantShrunk           bool
		wantErr              string
		wantSkip             string
	}{
		{name: "text shrinks", mode: "text", protocol: Text, wantShrunk: true},
		{name: "text ignores stderr noise", mode: "text-stderr", protocol: Text, wantShrunk: true},
		{name: "json shrinks", mode: "json", protocol: JSON, wantShrunk: true},
		{name: "rpc shrinks", mode: "rpc", protocol: RPC, wantShrunk: true},
		{name: "non-zero exit", mode: "exit1", protocol: Text, wantErr: "boom"},
		{name: "timeout", mode: "sleep", protocol: Text, wantErr: "timed out"},
		{name: "empty answer", mode: "empty", protocol: Text, wantErr: "empty text"},
		{name: "bigger answer ignored", mode: "grow", protocol: Text, wantSkip: "nothing shorter"},
		{name: "bad json", mode: "json-bad", protocol: JSON, wantErr: "bad json"},
		{name: "child error field", mode: "json-error", protocol: JSON, wantErr: "cannot parse"},
		{name: "wrong version", mode: "json-version", protocol: JSON, wantErr: "version 99"},
		{name: "child skip reason", mode: "json-skip", protocol: JSON, wantSkip: "not a log"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := helperConfig(tt.mode, tt.protocol)
			if tt.wantErr == "timed out" {
				cfg.Timeout = 300 * time.Millisecond
			}
			p := logPayload(t)
			orig := p.Text()
			err := newOpt(t, cfg).Optimize(t.Context(), p)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Text() != orig; got != tt.wantShrunk {
				t.Fatalf("changed = %v, want %v", got, tt.wantShrunk)
			}
			if tt.wantShrunk && (len(p.Text()) >= len(orig) || !strings.HasPrefix(p.Text(), "[")) {
				t.Fatalf("not shrunk by the child: %.80q", p.Text())
			}
			if !strings.Contains(p.Attrs[payload.AttrSkip], tt.wantSkip) {
				t.Fatalf("skip = %q, want %q", p.Attrs[payload.AttrSkip], tt.wantSkip)
			}
		})
	}
}

func TestMetadataAndEnv(t *testing.T) {
	t.Setenv("EXTOPT_SECRET", "hunter2")
	t.Setenv("EXTOPT_PASSED", "ok")

	p := payload.New(payload.CmdOutput, strings.Repeat("some output line\n", 50))
	p.Blocks[0].Type = "log/app"
	p.Set(payload.AttrCmd, "tail app.log")
	if err := newOpt(t, helperConfig("text-attr", Text)).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if got := p.Text(); got != "log/app|tail app.log|1" {
		t.Fatalf("child saw %q", got)
	}

	cfg := helperConfig("text-env", Text)
	cfg.PassEnv = []string{"EXTOPT_PASSED"}
	p = payload.New(payload.CmdOutput, strings.Repeat("some output line\n", 200))
	if err := newOpt(t, cfg).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	env := p.Text()
	if strings.Contains(env, "EXTOPT_SECRET") || !strings.Contains(env, "EXTOPT_PASSED=ok") || !strings.Contains(env, "EXTOPT_MODE=text-env") {
		t.Fatalf("child environment not scrubbed as configured:\n%s", env)
	}
}

func TestSelection(t *testing.T) {
	tests := []struct {
		name         string
		types, cmds  []string
		typ, cmd     string
		wantSelected bool
	}{
		{name: "empty selects all", typ: "log/app", cmd: "x", wantSelected: true},
		{name: "exact type", types: []string{"log/app"}, typ: "log/app", wantSelected: true},
		{name: "other type", types: []string{"log/app"}, typ: "log/nginx"},
		{name: "family", types: []string{"log/*"}, typ: "log/nginx", wantSelected: true},
		{name: "family root", types: []string{"log/*"}, typ: "log", wantSelected: true},
		{name: "family is not a prefix match", types: []string{"log/*"}, typ: "logs/x"},
		{name: "cmd glob", cmds: []string{"go test*"}, typ: "t", cmd: "go test ./...", wantSelected: true},
		{name: "cmd glob miss", cmds: []string{"go test*"}, typ: "t", cmd: "go build"},
		{name: "cmd middle star", cmds: []string{"npm * --json"}, typ: "t", cmd: "npm ls --json", wantSelected: true},
		{name: "types and cmds both", types: []string{"log/*"}, cmds: []string{"tail*"}, typ: "log/app", cmd: "cat x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Config{Types: tt.types, Cmds: tt.cmds}
			if got := c.wants(tt.typ, tt.cmd); got != tt.wantSelected {
				t.Fatalf("wants(%q, %q) = %v", tt.typ, tt.cmd, got)
			}
		})
	}
}

func TestSkipsUnselectedBlocks(t *testing.T) {
	cfg := helperConfig("exit1", Text)
	cfg.Types = []string{"test/*"}
	p := logPayload(t) // log/app: the failing child must not even run
	if err := newOpt(t, cfg).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Attrs[payload.AttrSkip], "external/t") {
		t.Fatalf("skip = %q", p.Attrs[payload.AttrSkip])
	}
}

// A child that dies is restarted once; a crash on every call is an error.
func TestRPCRestartsAfterCrash(t *testing.T) {
	cfg := helperConfig("rpc-crash-once", RPC)
	cfg.Env["EXTOPT_MARKER"] = t.TempDir() + "/crashed"
	o := newOpt(t, cfg)
	for range 2 {
		p := logPayload(t)
		if err := o.Optimize(t.Context(), p); err != nil || !strings.HasPrefix(p.Text(), "[") {
			t.Fatalf("Optimize = %v, text %.40q", err, p.Text())
		}
	}
}

func TestOutputLimit(t *testing.T) {
	cfg := helperConfig("grow", Text)
	cfg.MaxOutputBytes = 1000
	if err := newOpt(t, cfg).Optimize(t.Context(), logPayload(t)); err == nil || !strings.Contains(err.Error(), "max_output_bytes") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidate(t *testing.T) {
	ok := func() *Config { return helperConfig("text", Text) }
	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{"valid", func(*Config) {}, ""},
		{"no command", func(c *Config) { c.Command = nil }, "command"},
		{"unknown protocol", func(c *Config) { c.Protocol = "grpc" }, "json, rpc, text"},
		{"zero timeout", func(c *Config) { c.Timeout = 0 }, "timeout"},
		{"bad pattern", func(c *Config) { c.Types = []string{"[x"} }, "bad pattern"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := ok()
			tt.edit(c)
			err := c.Validate()
			if (tt.want == "") != (err == nil) || err != nil && !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate = %v, want %q", err, tt.want)
			}
		})
	}
}
