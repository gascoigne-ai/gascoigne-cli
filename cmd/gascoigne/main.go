// Command gascoigne optimizes what LLM agents read: CLI output, tool results, API traffic.
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/internal/config"
	"github.com/gascoigne-ai/gascoigne-cli/internal/registry"
	"github.com/gascoigne-ai/gascoigne-cli/internal/service"
	"github.com/gascoigne-ai/gascoigne-cli/mcptool"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/execreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

func main() {
	if len(os.Args) < 2 {
		printOverview(os.Stderr)
		os.Exit(2)
	}
	name := os.Args[1]
	switch name {
	case "-h", "-help", "--help":
		name = "help"
	case "-v", "-version", "--version":
		name = "version"
	}
	cmd, ok := lookup(name)
	if !ok {
		fmt.Fprintln(os.Stderr, "gascoigne:", unknownCommand(name))
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := cmd.run(ctx, os.Args[2:])
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return
	}
	var code exitCode
	if errors.As(err, &code) {
		os.Exit(int(code))
	}
	fmt.Fprintln(os.Stderr, "gascoigne:", err)
	os.Exit(1)
}

// common holds flags shared by commands that build the service.
type common struct {
	config  string
	verbose bool
	attrs   attrFlag
	sets    listFlag
	only    string
	disable string
	session string
	agent   string
	model   string
	// server: this command optimizes for an agent or user, so it makes sure
	// the background server (dashboard) runs.
	server bool
	// useServer: payloads go to the background server's warm pipeline when
	// it serves this build and ledger; else (or when it fails) the pipeline
	// is built here.
	useServer bool
	// pipeline, when set, is the only one this run builds: a remote run has
	// no ledger for the other pipelines' receivers (MCP) to start with.
	pipeline string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.config, "config", "", "config `file` (default: $GASCOIGNE_CONFIG, $XDG_CONFIG_HOME/gascoigne/config.yaml, built-in)")
	fs.BoolVar(&c.verbose, "v", false, "debug logs to stderr")
	c.attrs = attrFlag{}
	fs.Var(c.attrs, "attr", "payload attribute `k=v` (repeatable), e.g. -attr cmd='git status'")
	fs.Var(&c.sets, "set", "config override `path=yaml` (repeatable, after $GASCOIGNE_SET), e.g. -set optimizers.grep.max_per_file=50")
	fs.StringVar(&c.only, "only", "", "run only these optimizers, a comma `list` (default $GASCOIGNE_ONLY)")
	fs.StringVar(&c.disable, "disable", "", "skip these optimizers, a comma `list` (default $GASCOIGNE_DISABLE)")
	fs.StringVar(&c.session, "session", "", "agent session `id` for stats (default $GASCOIGNE_SESSION)")
	fs.StringVar(&c.agent, "agent", "", "agent `name` for stats, e.g. claude-code (default $GASCOIGNE_AGENT; MCP: client name)")
	fs.StringVar(&c.model, "model", "", "LLM `model` the output is for (default $GASCOIGNE_MODEL)")
}

// identity is session, agent and model from flags, else env.
func (c *common) identity() mcptool.Identity {
	return mcptool.Identity{
		Session: c.sessionID(),
		Agent:   cmp.Or(c.agent, os.Getenv("GASCOIGNE_AGENT")),
		Model:   cmp.Or(c.model, os.Getenv("GASCOIGNE_MODEL")),
	}
}

func (c *common) sessionID() string { return cmp.Or(c.session, os.Getenv("GASCOIGNE_SESSION")) }

// overrides merges env (per agent, set once in its environment) with flags (per run).
func (c *common) overrides() config.Overrides {
	return config.Overrides{
		Set:     append(splitList(os.Getenv("GASCOIGNE_SET"), ";\n"), c.sets...),
		Only:    splitList(cmp.Or(c.only, os.Getenv("GASCOIGNE_ONLY")), ","),
		Disable: splitList(cmp.Or(c.disable, os.Getenv("GASCOIGNE_DISABLE")), ","),
	}
}

func splitList(s, seps string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return strings.ContainsRune(seps, r) }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, " ") }
func (l *listFlag) Set(s string) error { *l = append(*l, s); return nil }

// start builds and starts the service; callers must Shutdown it.
func (c *common) start(ctx context.Context) (*service.Service, error) {
	data, err := configData(c.config)
	if err != nil {
		return nil, err
	}
	cfg, err := config.ParseWith(data, c.overrides())
	if err != nil {
		return nil, err
	}
	if pc, ok := cfg.Pipelines[c.pipeline]; ok {
		cfg.Pipelines = map[string]config.Pipeline{c.pipeline: pc}
	}
	level := slog.LevelWarn
	if c.verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	id := c.identity()
	for k, v := range map[string]string{payload.AttrSession: id.Session, payload.AttrAgent: id.Agent, payload.AttrModel: id.Model} {
		if v != "" {
			c.attrs[k] = v
		}
	}
	opts := service.Options{Logger: log, Attrs: c.attrs}
	if c.useServer {
		opts.Remote = c.remote(data, cfg, log)
	}
	svc, err := service.New(ctx, cfg, registry.Factories(), opts)
	if err != nil {
		return nil, err
	}
	if err := svc.Start(ctx); err != nil {
		return svc, err
	}
	if l, ok := svc.Ledger().(*ledger.Ledger); ok && c.server {
		// Never fails the command: the dashboard is a convenience.
		if err := ensureServer(ctx, serverAddr(), l.Path(), false); err != nil {
			log.Debug("server not started", "err", err)
		}
	}
	return svc, nil
}

func (c *common) load() (*config.Config, error) {
	data, err := configData(c.config)
	if err != nil {
		return nil, err
	}
	return config.ParseWith(data, c.overrides())
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
	fs := newFlags("run")
	c.register(fs)
	raw := fs.Bool("raw", false, "bypass optimization, output verbatim")
	pipeline := fs.String("pipeline", "cli", "`pipeline` to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *raw {
		c.attrs[payload.AttrRaw] = "true"
	}
	c.useServer, c.pipeline = true, *pipeline
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
	fs := newFlags("exec")
	c.register(fs)
	raw := fs.Bool("raw", false, "bypass optimization, output verbatim")
	pipeline := fs.String("pipeline", "exec", "`pipeline` to run")
	watch := fs.Bool("watch", false, "the command rebuilds until stopped (tsc -w, webpack --watch): print each cycle as it ends")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() == 0 {
		return errors.New("exec: want a command: gascoigne exec [flags] -- cmd args...")
	}
	if *raw {
		c.attrs[payload.AttrRaw] = "true"
	}
	c.server, c.useServer, c.pipeline = true, true, *pipeline
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	inv := &execreceiver.Invocation{Args: fs.Args(), Watch: *watch}
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
	return serve(ctx, "mcp-proxy", args, func(ctx context.Context, command []string) context.Context {
		return mcpproxyreceiver.WithLazy(mcpproxyreceiver.With(ctx, command), lazyChoices())
	})
}

// lazyChoices are the tools to load on request, by server id. GASCOIGNE_MCP_LAZY=off
// ignores them for a run, without touching the saved choices.
func lazyChoices() map[string][]string {
	if os.Getenv("GASCOIGNE_MCP_LAZY") == "off" {
		return nil
	}
	return readPrefs().LazyTools
}

// serve runs one long-lived pipeline; with sets per-run receiver input from positional args.
func serve(ctx context.Context, pipeline string, args []string, with func(context.Context, []string) context.Context) error {
	var c common
	fs := newFlags(pipeline)
	c.register(fs)
	name := fs.String("pipeline", pipeline, "`pipeline` to run")
	if err := fs.Parse(args); err != nil {
		return err
	}
	c.server = true
	svc, err := c.start(ctx)
	if err != nil {
		return err
	}
	run := mcptool.WithIdentity(with(ctx, fs.Args()), c.identity())
	return errors.Join(svc.Run(run, *name), svc.Shutdown(context.WithoutCancel(ctx)))
}

func cmdConfig(_ context.Context, args []string) error {
	var c common
	fs := newFlags("config")
	c.register(fs)
	upgrade := fs.Bool("upgrade", false, "rewrite the config file on this version's template, keeping your settings (old file: .bak)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *upgrade {
		return upgradeCmd(c.config)
	}
	cfg, err := c.load()
	if err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}

func cmdShow(_ context.Context, args []string) error {
	fs := newFlags("show")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("show: want <file>[:symbol]")
	}
	path, symbol := fs.Arg(0), ""
	if i := strings.LastIndexByte(path, ':'); i > 0 {
		if _, err := os.Stat(path); err != nil {
			path, symbol = path[:i], path[i+1:]
		}
	}
	text, err := mcptool.Show(path, symbol)
	if err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, text)
	return err
}
