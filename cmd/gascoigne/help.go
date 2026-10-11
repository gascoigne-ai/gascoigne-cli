package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/version"
)

// command is one subcommand: how it runs and how help presents it.
type command struct {
	name     string
	args     string // after the name, e.g. "[flags] -- <cmd> [args]"
	short    string // args in the overview when they differ: no "[flags]", shorter
	summary  string // one line for the overview
	group    string // overview section; "" keeps it out of the overview
	about    string // longer text for its own help page
	examples []string
	run      func(ctx context.Context, args []string) error
}

const (
	groupAgents   = "Agents"
	groupOptimize = "Optimize"
	groupSavings  = "Savings and originals"
	groupServer   = "Dashboard"
	groupDev      = "Config and development"
)

var groups = []string{groupAgents, groupOptimize, groupSavings, groupServer, groupDev}

// commandList is filled in init: the command funcs refer back to help.
var commandList []command

func init() {
	commandList = []command{
		{name: "wrap", args: "[-print] <agent> [agent args...]", short: "<agent> [args...]", group: groupAgents,
			summary: "start an agent with gascoigne registered for this run",
			about: "Starts the agent with gascoigne's hook and MCP server for this run only; no config file\n" +
				"is touched. Agents: claude, codex, gemini, opencode, pi. -print shows the config that registers it for good\n" +
				"(also for cursor) instead.",
			examples: []string{"gascoigne wrap claude", "gascoigne wrap codex --model o3", "gascoigne wrap -print cursor"},
			run:      cmdWrap},
		{name: "install", args: "[-g] [agent...]", group: groupAgents,
			summary: "register with agents for good: this project, -g every project",
			about: "Writes gascoigne's hook and MCP server into the agents' config files: the project's\n" +
				"(.claude/, .mcp.json, .codex/, .cursor/, .pi/), or with -g the user's. Earlier gascoigne\n" +
				"entries are replaced and changed files backed up as <file>.gascoigne-bak.\n" +
				"Agents: claude, codex, cursor, gemini, opencode, pi (default: those found on PATH).",
			examples: []string{"gascoigne install", "gascoigne install -g", "gascoigne install claude codex"},
			run:      cmdInstall},
		{name: "unwrap", args: "[agent...]", group: groupAgents,
			summary:  "remove gascoigne from agents' config files",
			about:    "Removes exactly what install or wrap -print added, from user and project configs\n(default: every agent). Other settings keep their order; originals are backed up.",
			examples: []string{"gascoigne unwrap", "gascoigne unwrap cursor"},
			run:      cmdUnwrap},
		{name: "hook", args: "<agent>",
			summary: "agent hook handler (called by the agent)",
			about:   "Reads a hook event on stdin and answers for the agent. Never fails the tool call.",
			run:     cmdHook},

		{name: "exec", args: "[flags] -- <cmd> [args...]", short: "-- <cmd> [args...]", group: groupOptimize,
			summary: "run a command, print its optimized output, keep its exit code",
			about: "Runs the command, merges stdout and stderr, optimizes the result and prints it with a\n" +
				"hint naming the original: gascoigne raw <id> prints it byte-exact.",
			examples: []string{"gascoigne exec -- go test ./...", "gascoigne exec -raw -- git diff", "gascoigne exec -attr query=timeout -- kubectl logs api"},
			run:      cmdExec},
		{name: "run", args: "[flags]", group: groupOptimize,
			summary:  "optimize stdin to stdout",
			about:    "Optimizes text on stdin. Hints like -attr cmd=… or -attr file=… help detect what it is.",
			examples: []string{"kubectl get pods -o yaml | gascoigne run", "gascoigne run -attr file=package-lock.json < package-lock.json"},
			run:      cmdRun},
		{name: "mcp", args: "[flags]", group: groupOptimize,
			summary:  "MCP server on stdio: fetch, compress, retrieve, stats",
			about:    "fetch reads web pages as markdown and compresses API JSON; compress takes any text;\nretrieve returns originals; stats reports savings.",
			examples: []string{"claude mcp add gascoigne -- gascoigne mcp"},
			run:      cmdMCP},
		{name: "mcp-proxy", args: "[flags] -- <server cmd>", short: "-- <server>", group: groupOptimize,
			summary:  "MCP proxy optimizing another server's tool results",
			about:    "Mirrors the downstream server's tools, optimizes their text results and adds\ngascoigne_retrieve. Non-text content passes untouched.",
			examples: []string{"gascoigne mcp-proxy -- npx -y @modelcontextprotocol/server-github"},
			run:      cmdMCPProxy},

		{name: "gain", args: "[flags]", group: groupSavings,
			summary:  "token savings report",
			examples: []string{"gascoigne gain", "gascoigne gain -by command -since 7d", "gascoigne gain -session claude-20261010-111234 -json"},
			run:      cmdGain},
		{name: "raw", args: "<id>", group: groupSavings,
			summary:  "print an original byte-exact (marks the source as failing)",
			examples: []string{"gascoigne raw 9f1c2e7a0b3d4c55"},
			run:      cmdRaw},
		{name: "fail", args: "<key|raw id>", group: groupSavings,
			summary:  "mark a source as badly optimized",
			examples: []string{"gascoigne fail 'cmd:go test ./...'"},
			run:      cmdFail},
		{name: "forget", args: "<key>", group: groupSavings,
			summary:  "clear a source's failure history",
			examples: []string{"gascoigne forget 'cmd:go test ./...'"},
			run:      cmdForget},
		{name: "reset", args: "[flags]", group: groupSavings,
			summary:  "clear savings stats (-all: also originals and failures)",
			about:    "Deletes recorded runs. Filters come from flags only, never from GASCOIGNE_* variables.",
			examples: []string{"gascoigne reset -session claude-20261010-111234", "gascoigne reset -agent codex -since 24h", "gascoigne reset -all"},
			run:      cmdReset},
		{name: "flush", args: "[-older 7d]", group: groupSavings,
			summary:  "delete stored originals, the cache behind raw ids",
			about:    "Deletes stored originals and vacuums the ledger. Runs and savings stay.",
			examples: []string{"gascoigne flush", "gascoigne flush -older 2h"},
			run:      cmdFlush},
		{name: "probe", args: "[flags]", group: groupSavings,
			summary: "check recorded runs for error lines and artifacts their output lost",
			about: "Compares stored originals with their optimized output: error lines, artifacts (paths,\n" +
				"URLs, hashes) and key=number pairs still readable, per family, and the runs that lost most.",
			examples: []string{"gascoigne probe", "gascoigne probe -since 7d -worst 20", "gascoigne probe -json"},
			run:      cmdProbe},
		{name: "replay", args: "[flags]", group: groupSavings,
			summary: "replay recorded agent sessions through the LLM API proxy",
			about: "Rebuilds the requests of Claude Code transcripts and sends them through the proxy against a\n" +
				"stand-in upstream (nothing leaves the machine, throwaway ledger): tool-result savings per tool,\n" +
				"and whether messages sent again stay byte-identical for the prompt cache.",
			examples: []string{"gascoigne replay", "gascoigne replay -sessions 50 -json"},
			run:      cmdReplay},
		{name: "tokens", args: "[-model list] [file]", group: groupSavings,
			summary: "count text as each model family does, and price it",
			about: "Counts a file (or stdin) with every tokenizer family gascoigne knows, then prices one\n" +
				"read of it and its cost while it stays in an agent's context, per model.",
			examples: []string{"gascoigne tokens README.md", "go test ./... 2>&1 | gascoigne tokens -model claude-opus-5-5,gpt-5.5"},
			run:      cmdTokens},

		{name: "dashboard", args: "[flags]", group: groupServer,
			summary: "start the server if needed, open the dashboard in a browser",
			about:   "Prints the dashboard address and opens it in the default browser; -open=false only prints it.",
			run:     cmdDashboard},
		{name: "server", args: "[run|start|stop|status] [flags]", short: "[start|stop|status]", group: groupServer,
			summary: "the background server behind the dashboard, one per user",
			about: "Commands that optimize start it when it is not answering; exec, run and the codex hook\n" +
				"then optimize through it, and it serves the LLM proxy under /v1/. It restarts itself when\n" +
				"its binary changes and exits after 30m without a request (run -idle, GASCOIGNE_SERVER_IDLE,\n" +
				"0 never); the next command or agent prompt starts it again. GASCOIGNE_SERVER=off keeps\n" +
				"every command off it.",
			examples: []string{"gascoigne server status", "gascoigne server stop"},
			run:      cmdServer},

		{name: "config", args: "[flags]", group: groupDev,
			summary: "print the effective config, overrides applied; -upgrade moves your settings to a new version",
			about: "Your config file (~/.config/gascoigne/config.yaml) lists every setting commented out. After an\n" +
				"upgrade, a file that sets nothing is refreshed by itself; one with settings is left alone, and\n" +
				"config -upgrade rewrites it on the new template, keeps your settings and drops the ones this\n" +
				"version no longer has (the old file stays as config.yaml.bak).",
			examples: []string{"gascoigne config -set optimizers.grep.max_per_file=50", "gascoigne config -upgrade"},
			run:      cmdConfig},
		{name: "components", group: groupDev,
			summary: "list built-in components",
			run:     cmdComponents},
		{name: "eval", args: "[flags]", group: groupDev,
			summary:  "token savings on the eval fixtures",
			examples: []string{"gascoigne eval", "gascoigne eval -show rg", "gascoigne eval -update"},
			run:      cmdEval},
		{name: "help", args: "[command]", group: groupDev,
			summary: "this overview, or a command's flags and examples",
			run:     cmdHelp},
		{name: "version", group: groupDev,
			summary:  "release and the git commit it was built from",
			examples: []string{"gascoigne version", "gascoigne --version"},
			run:      cmdVersion},
	}
}

func lookup(name string) (command, bool) {
	i := slices.IndexFunc(commandList, func(c command) bool { return c.name == name })
	if i < 0 {
		return command{}, false
	}
	return commandList[i], true
}

func cmdHelp(_ context.Context, args []string) error {
	if len(args) == 0 {
		printOverview(os.Stdout)
		return nil
	}
	c, ok := lookup(args[0])
	if !ok {
		return unknownCommand(args[0])
	}
	if c.name == "help" {
		printCommand(os.Stdout, c.name, newFlags(c.name))
		return nil
	}
	// The command defines its flags, so let it print its page.
	return c.run(context.Background(), []string{"-h"})
}

func unknownCommand(name string) error {
	msg := fmt.Sprintf("unknown command %q", name)
	if s := suggest(name); s != "" {
		msg += fmt.Sprintf(", did you mean %q?", s)
	}
	return fmt.Errorf("%s (gascoigne help lists commands)", msg)
}

// suggest is the command a typo or prefix most likely meant, if any is close.
func suggest(name string) string {
	best, bestDist := "", 3
	for _, c := range commandList {
		d := distance(name, c.name)
		if len(name) >= 2 && strings.HasPrefix(c.name, name) {
			d = 1
		}
		if d < bestDist {
			best, bestDist = c.name, d
		}
	}
	return best
}

// distance is the Levenshtein edit distance.
func distance(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// painterFor colors help on a terminal without NO_COLOR.
func painterFor(w io.Writer) painter {
	f, ok := w.(*os.File)
	if !ok {
		return painter{}
	}
	fi, err := f.Stat()
	return painter{err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""}
}

func printOverview(w io.Writer) {
	p := painterFor(w)
	fmt.Fprintf(w, "%s %s shrinks what LLM agents read, keeping every original retrievable.\n\n", p.paint(bold, "gascoigne"), p.paint(dim, version.String()))
	fmt.Fprintf(w, "%s\n  gascoigne %s %s\n", p.paint(bold, "USAGE"), p.paint(cyan, "<command>"), p.paint(dim, "[flags]"))
	const width = 26
	for _, g := range groups {
		fmt.Fprintf(w, "\n%s\n", p.paint(bold, strings.ToUpper(g)))
		for _, c := range commandList {
			if c.group != g {
				continue
			}
			args := c.short
			if args == "" {
				args = strings.TrimSpace(strings.ReplaceAll(c.args, "[flags]", ""))
			}
			plain, styled := c.name, p.paint(cyan, c.name)
			if args != "" {
				plain, styled = plain+" "+args, styled+" "+p.paint(dim, args)
			}
			fmt.Fprintf(w, "  %s%s  %s\n", styled, strings.Repeat(" ", max(width-len(plain), 0)), c.summary)
		}
	}
	fmt.Fprintf(w, "\n%s\n", p.paint(bold, "GET STARTED"))
	for _, e := range []string{
		"gascoigne install -g              # every agent, every project",
		"gascoigne exec -- go test ./...   # one command by hand",
		"gascoigne gain                    # where the savings come from",
		"gascoigne dashboard               # live view in the browser",
	} {
		fmt.Fprintf(w, "  %s\n", example(p, e))
	}
	fmt.Fprintf(w, "\nRun %s for a command's flags and examples. Docs: docs/README.md\n", p.paint(cyan, "gascoigne help <command>"))
}

// example dims a trailing "# comment".
func example(p painter, e string) string {
	cmd, comment, ok := strings.Cut(e, "#")
	if !ok {
		return e
	}
	return cmd + p.paint(dim, "#"+comment)
}

// commonFlags are the flags every service-building command shares (common.register).
var commonFlags = []string{"config", "v", "attr", "set", "only", "disable", "session", "agent", "model"}

// newFlags is a flag set whose -h prints the command's help page to
// stdout; flag errors still go to stderr.
func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() { printCommand(os.Stdout, name, fs) }
	return fs
}

func printCommand(w io.Writer, name string, fs *flag.FlagSet) {
	p := painterFor(w)
	c, _ := lookup(strings.Fields(name)[0])
	fmt.Fprintf(w, "%s %s %s %s\n", p.paint(bold, "gascoigne"), p.paint(bold+";"+cyan, c.name), p.paint(dim, "—"), c.summary)
	fmt.Fprintf(w, "\n%s\n  %s\n", p.paint(bold, "USAGE"), strings.TrimSpace("gascoigne "+p.paint(cyan, c.name)+" "+p.paint(dim, c.args)))
	if c.about != "" {
		fmt.Fprintf(w, "\n%s\n", indent(c.about))
	}
	var own, shared []*flag.Flag
	fs.VisitAll(func(f *flag.Flag) {
		if slices.Contains(commonFlags, f.Name) {
			shared = append(shared, f)
			return
		}
		own = append(own, f)
	})
	printFlags(w, p, "FLAGS", own)
	if len(c.examples) > 0 {
		fmt.Fprintf(w, "\n%s\n", p.paint(bold, "EXAMPLES"))
		for _, e := range c.examples {
			fmt.Fprintf(w, "  %s\n", e)
		}
	}
	printFlags(w, p, "SHARED FLAGS", shared)
}

func printFlags(w io.Writer, p painter, title string, flags []*flag.Flag) {
	if len(flags) == 0 {
		return
	}
	type row struct{ name, usage string }
	rows := make([]row, len(flags))
	width := 0
	for i, f := range flags {
		kind, usage := flag.UnquoteUsage(f)
		name := "-" + f.Name
		if kind != "" {
			name += " " + kind
		}
		if f.DefValue != "" && f.DefValue != "false" && f.DefValue != "0" && f.DefValue != "[]" && f.DefValue != "map[]" && !strings.Contains(usage, "default") {
			usage += " (default " + f.DefValue + ")"
		}
		rows[i] = row{name, usage}
		width = max(width, len(name))
	}
	width = min(width, 18)
	fmt.Fprintf(w, "\n%s\n", p.paint(bold, title))
	for _, r := range rows {
		pad := width - len(r.name)
		lead := "  " + p.paint(cyan, r.name)
		if pad < 0 { // too long: description on the next line
			fmt.Fprintf(w, "%s\n", lead)
			lead, pad = "  ", width
		}
		lines := wrap(r.usage, 100-width-4)
		fmt.Fprintf(w, "%s%s  %s\n", lead, strings.Repeat(" ", pad), lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(w, "  %s  %s\n", strings.Repeat(" ", width), l)
		}
	}
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// wrap breaks s into lines of at most n bytes at spaces.
func wrap(s string, n int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(s) {
		if line != "" && len(line)+1+len(word) > n {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}
	return append(lines, line)
}

func cmdVersion(_ context.Context, args []string) error {
	if err := newFlags("version").Parse(args); err != nil {
		return err
	}
	fmt.Println("gascoigne", version.String())
	return nil
}
