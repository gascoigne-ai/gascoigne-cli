// Command gascoigne optimizes what LLM agents read: CLI output, tool results, API traffic.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/lukaszb/gascoigne/configs"
	"github.com/lukaszb/gascoigne/internal/config"
	"github.com/lukaszb/gascoigne/internal/registry"
	"github.com/lukaszb/gascoigne/internal/service"
	"github.com/lukaszb/gascoigne/payload"
	"github.com/lukaszb/gascoigne/receiver/execreceiver"
	"github.com/lukaszb/gascoigne/receiver/mcpproxyreceiver"
)

const usage = `usage: gascoigne <command> [flags]

commands:
  run         run a pipeline (default cli: stdin → optimized stdout)
  exec -- <cmd> [args]  run cmd, print its optimized output, keep its exit code
  mcp         MCP server on stdio: fetch, compress, retrieve, stats
  mcp-proxy -- <server cmd>  MCP proxy optimizing another server's tool results
  raw <id>    print original payload; marks its source as badly optimized
  fail <key|raw_id>  mark a source as badly optimized
  forget <key>       clear failure history for a source
  gain        token savings report
  eval        benchmark pipeline on fixtures
  components  list built-in components

run 'gascoigne <command> -h' for flags`

var commands = map[string]func(ctx context.Context, args []string) error{
	"run":        cmdRun,
	"exec":       cmdExec,
	"mcp":        cmdMCP,
	"mcp-proxy":  cmdMCPProxy,
	"raw":        cmdRaw,
	"fail":       cmdFail,
	"forget":     cmdForget,
	"gain":       cmdGain,
	"eval":       cmdEval,
	"components": cmdComponents,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	cmd, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command %q\n%s\n", os.Args[1], usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd(ctx, os.Args[2:]); err != nil {
		var code exitCode
		if errors.As(err, &code) {
			os.Exit(int(code))
		}
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "gascoigne:", err)
		}
		os.Exit(1)
	}
}

// common holds flags shared by commands that build the service.
type common struct {
	config  string
	verbose bool
	attrs   attrFlag
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "", "config file (default: $GASCOIGNE_CONFIG, $XDG_CONFIG_HOME/gascoigne/config.yaml, built-in)")
	fs.BoolVar(&c.verbose, "v", false, "debug logs to stderr")
	c.attrs = attrFlag{}
	fs.Var(c.attrs, "attr", "payload attribute k=v (repeatable), e.g. -attr cmd='git status'")
}

// start builds and starts the service; callers must Shutdown it.
func (c *common) start(ctx context.Context) (*service.Service, error) {
	cfg, err := loadConfig(c.config)
	if err != nil {
		return nil, err
	}
	level := slog.LevelWarn
	if c.verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	svc, err := service.New(ctx, cfg, registry.Factories(), service.Options{Logger: log, Attrs: c.attrs})
	if err != nil {
		return nil, err
	}
	return svc, svc.Start(ctx)
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		path = os.Getenv("GASCOIGNE_CONFIG")
	}
	if path == "" {
		if dir, err := os.UserConfigDir(); err == nil {
			if p := filepath.Join(dir, "gascoigne", "config.yaml"); fileExists(p) {
				path = p
			}
		}
	}
	if path == "" {
		return config.Parse(configs.Default)
	}
	return config.Load(path)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

type attrFlag map[string]string

func (a attrFlag) String() string { return fmt.Sprint(map[string]string(a)) }

func (a attrFlag) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok || k == "" {
		return fmt.Errorf("want k=v, got %q", s)
	}
	a[k] = v
	return nil
}

func cmdRun(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	c.register(fs)
	raw := fs.Bool("raw", false, "bypass optimization, output verbatim")
	pipeline := fs.String("pipeline", "cli", "pipeline to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *raw {
		c.attrs[payload.AttrRaw] = "true"
	}
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	return errors.Join(svc.Run(ctx, *pipeline), svc.Shutdown(context.WithoutCancel(ctx)))
}

// exitCode makes main exit with the wrapped command's status.
type exitCode int

func (c exitCode) Error() string { return fmt.Sprintf("exit status %d", int(c)) }

func cmdExec(ctx context.Context, args []string) error {
	var c common
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	c.register(fs)
	raw := fs.Bool("raw", false, "bypass optimization, output verbatim")
	pipeline := fs.String("pipeline", "exec", "pipeline to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("exec: want a command: gascoigne exec [flags] -- cmd args...")
	}
	if *raw {
		c.attrs[payload.AttrRaw] = "true"
	}
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	inv := &execreceiver.Invocation{Args: fs.Args()}
	runErr := svc.Run(execreceiver.With(ctx, inv), *pipeline)
	if err := errors.Join(runErr, svc.Shutdown(context.WithoutCancel(ctx))); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
	}
	if inv.ExitCode != 0 || runErr != nil {
		return exitCode(max(inv.ExitCode, 1))
	}
	return nil
}

func cmdMCP(ctx context.Context, args []string) error {
	return serve(ctx, "mcp", args, func(ctx context.Context, _ []string) context.Context { return ctx })
}

func cmdMCPProxy(ctx context.Context, args []string) error {
	return serve(ctx, "mcp-proxy", args, mcpproxyreceiver.With)
}

// serve runs one long-lived pipeline; with sets per-run receiver input from positional args.
func serve(ctx context.Context, pipeline string, args []string, with func(context.Context, []string) context.Context) error {
	var c common
	fs := flag.NewFlagSet(pipeline, flag.ContinueOnError)
	c.register(fs)
	name := fs.String("pipeline", pipeline, "pipeline to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	return errors.Join(svc.Run(with(ctx, fs.Args()), *name), svc.Shutdown(context.WithoutCancel(ctx)))
}
