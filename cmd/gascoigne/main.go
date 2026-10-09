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
)

const usage = `usage: gascoigne <command> [flags]

commands:
  run         run pipelines (stdin → optimized stdout)
  raw <id>    print original payload; marks its source as badly optimized
  fail <key|raw_id>  mark a source as badly optimized
  forget <key>       clear failure history for a source
  gain        token savings report
  eval        benchmark pipeline on fixtures
  components  list built-in components

run 'gascoigne <command> -h' for flags`

var commands = map[string]func(ctx context.Context, args []string) error{
	"run":        cmdRun,
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
	return errors.Join(svc.Run(ctx), svc.Shutdown(context.WithoutCancel(ctx)))
}
