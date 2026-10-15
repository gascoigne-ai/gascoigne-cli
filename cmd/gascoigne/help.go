package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
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
	// experimental commands exist only when the config's experimental list
	// or GASCOIGNE_EXPERIMENTAL (a comma list) names them, or says "all".
	experimental bool
}

// available reports whether c can run: stable, or experimental and enabled.
func available(c command) bool { return !c.experimental || experimentalOn(c.name) }

// experimentalOn reports whether an experimental feature is enabled: named
// (or "all") in $GASCOIGNE_EXPERIMENTAL, the config's experimental list or
// the choices saved by gascoigne setup.
func experimentalOn(name string) bool {
	on := append(splitList(os.Getenv("GASCOIGNE_EXPERIMENTAL"), ","), configExperimental()...)
	on = append(on, readPrefs().Experimental...)
	return slices.Contains(on, "all") || slices.Contains(on, name)
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
		{name: "install", args: "[-g] [-guide off|short|full] [-permissions none|local|all] [-autosync on|off] [agent...]", group: groupAgents,
			summary: "register with agents for good: this project, -g every project",
			about: "Writes gascoigne's hook and MCP server into the agents' config files: the project's\n" +
				"(.claude/, .mcp.json, .codex/, .cursor/, .pi/), or with -g the user's. Earlier gascoigne\n" +
				"entries are replaced and changed files backed up as <file>.gascoigne-bak.\n" +
				"-guide sets how much usage text agents get (saved; $GASCOIGNE_GUIDE overrides it).\n" +
				"-permissions lets claude run gascoigne's own tools without asking (saved; unwrap removes the rules).\n" +
				"-autosync on lets agent hooks update user-scope installs after a gascoigne upgrade; gascoigne upgrade does it now.\n" +
				"Agents: claude, codex, cursor, gemini, opencode, pi (default: those found on PATH).",
			examples: []string{"gascoigne install", "gascoigne install -g", "gascoigne install claude codex"},
			run:      cmdInstall},
		{name: "setup", args: "[-y] [-all] [-t] [-experimental]", group: groupAgents,
			summary: "answer the open questions about permissions, hook updates and new agents",
			about: "gascoigne asks before it lets claude run its own tools without asking, updates installed hooks\n" +
				"by itself or sets up an agent it found. setup opens the page in your browser (dashboard /setup);\n" +
				"where none can open (ssh, CI) or with -t it asks here. -y takes the suggested answers, -all asks\n" +
				"the answered questions again, -experimental asks only about the experimental features.",
			examples: []string{"gascoigne setup", "gascoigne setup -t", "gascoigne setup -experimental"},
			run:      cmdSetup},
		{name: "upgrade", args: "[-n]", group: groupAgents,
			summary: "after installing a new version: update hooks, config and open choices",
			about: "One place for what a new gascoigne needs done: writes stale or lost hooks, rules and commands of\n" +
				"your installs again (all projects, and this project; others are named), moves your config.yaml\n" +
				"onto this version's template keeping your settings (previous file kept as .bak), and opens the\n" +
				"browser page for choices still waiting. -n only lists what would change.",
			examples: []string{"gascoigne upgrade", "gascoigne upgrade -n"},
			run:      cmdUpgrade},
		{name: "doctor", args: "", group: groupAgents,
			summary: "check installs, hooks, server and saved answers",
			about: "Reports, per recorded install, whether the agent's config still holds gascoigne's hook and\n" +
				"whether a newer gascoigne changed what install writes; also the background server, the guide\n" +
				"level and open questions. It changes nothing; gascoigne upgrade repairs what it finds. Exits 1\n" +
				"when there is a problem.",
			examples: []string{"gascoigne doctor"},
			run:      cmdDoctor},
		{name: "unwrap", args: "[agent...]", group: groupAgents,
			summary:  "remove gascoigne from agents' config files",
			about:    "Removes exactly what install or wrap -print added, from user and project configs\n(default: every agent). Other settings keep their order; originals are backed up.",
			examples: []string{"gascoigne unwrap", "gascoigne unwrap cursor"},
			run:      cmdUnwrap},
		{name: "uninstall", args: "[-y]", group: groupAgents,
			summary:  "remove gascoigne: agent configs, server, ledger, config, caches and the binary",
			about:    "Runs unwrap for every agent (user and this project's configs), stops the server, deletes\ngascoigne's config, data (ledger), cache and state dirs, then the binary. Lists it all and\nasks first; -y skips the question. Run unwrap in other projects installed without -g first.",
			examples: []string{"gascoigne uninstall", "gascoigne uninstall -y"},
			run:      cmdUninstall},
		{name: "hook", args: "<agent>",
			summary: "agent hook handler (called by the agent)",
			about:   "Reads a hook event on stdin and answers for the agent. Never fails the tool call.",
			run:     cmdHook},

		{name: "show", args: "<file>[:symbol]", group: groupOptimize,
			summary:  "print one function, method or type of a source file, or its outline",
			about:    "With a symbol (Name, Type.Method), prints that definition and its doc comment with line\nnumbers; without one, each definition's signature line and line range.",
			examples: []string{"gascoigne show internal/service/pipeline.go:Process", "gascoigne show app/models.py"},
			run:      cmdShow},
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
			summary:  "MCP server on stdio: fetch, compress, show, retrieve, stats",
			about:    "fetch reads web pages as markdown and compresses API JSON; compress takes any text;\nshow reads one definition of a source file; retrieve returns originals; stats reports savings.",
			examples: []string{"claude mcp add gascoigne -- gascoigne mcp"},
			run:      cmdMCP},
		{name: "mcp-proxy", args: "[flags] -- <server cmd>", short: "-- <server>", group: groupOptimize,
			summary:  "MCP proxy optimizing another server's tool results",
			about:    "Mirrors the downstream server's tools, optimizes their text results and adds\ngascoigne_retrieve. Non-text content passes untouched.\nA remote server (HTTP or SSE) is given with -url instead of a command.",
			examples: []string{"gascoigne mcp-proxy -- npx -y @modelcontextprotocol/server-github", "gascoigne mcp-proxy -url https://mcp.example.com/mcp -header \"Authorization: Bearer $TOKEN\""},
			run:      cmdMCPProxy},
		{name: "mcp-login", args: "[-logout] <url>", group: groupOptimize,
			summary: "log in to a remote MCP server that uses OAuth ahead of time, or forget the login",
			about: "mcp-proxy -url asks for a login itself (its gascoigne_login tool); this does it from the terminal,\n" +
				"e.g. on a headless machine. Opens the authorization page in a browser (the address is printed too)\n" +
				"and saves the tokens (0600, state dir); they are refreshed on use. -logout forgets them.",
			examples: []string{"gascoigne mcp-login https://mcp.example.com/mcp", "gascoigne mcp-login -logout https://mcp.example.com/mcp"},
			run:      cmdMCPLogin},

		{name: "gain", args: "[flags]", group: groupSavings,
			summary:  "token savings report",
			examples: []string{"gascoigne gain", "gascoigne gain -by command -since 7d", "gascoigne gain -brief", "gascoigne gain -show agent,model -top 3", "gascoigne gain -by command -sort runs -min-tokens 1000"},
			run:      cmdGain},
		{name: "raw", args: "<id> [-lines a-b|-grep re|-query words]", group: groupSavings,
			summary:  "print an original byte-exact (marks the source as failing), or part of it",
			examples: []string{"gascoigne raw 9f1c2e7a", "gascoigne raw 9f1c2e7a -grep 'FAIL|panic' -C 2"},
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
		{name: "learn", args: "[-since 30d] [-apply|-remove] [-target FILE]", group: groupSavings, experimental: true,
			summary: "propose instruction lines from habits that cost context",
			about: "Looks for commands an agent runs again for output gascoigne already shortened (the rest is one\n" +
				"gascoigne raw away) and commands whose large output nothing can shorten, in at least two\n" +
				"sessions. Prints the lines; -apply writes them as a marked block to CLAUDE.local.md (or\n" +
				"-target), -remove takes the block out again. Nothing is written without -apply.",
			examples: []string{"gascoigne learn", "gascoigne learn -apply", "gascoigne learn -apply -target AGENTS.md"},
			run:      cmdLearn},
		{name: "context", args: "[-since 14d] [-text] [-json] [-open=false]", group: groupSavings,
			summary: "how big conversations get, where the money goes, what compacting earlier would save",
			about: "Reads Claude Code's own transcripts (they hold the usage of every request, with a session id\n" +
				"the LLM proxy never sees) and shows, at API list prices, how the cost splits by the size of the\n" +
				"conversation each request re-read, your biggest sessions, and what compacting at 150k, 250k and\n" +
				"400k tokens would have saved. Opens a page in the dashboard; -text prints the table, -json the\n" +
				"report. Nothing leaves the machine and nothing is changed; compacting saves tokens, not detail.",
			examples: []string{"gascoigne context", "gascoigne context -since 30d -text"},
			run:      cmdContext},
		{name: "mcp-review", args: "[-text] [-json] [-open=false]", group: groupSavings,
			summary: "which MCP servers add tokens to the tool list and which are used",
			about: "Opens a page in the dashboard (the browser is the first choice) with each MCP server's tool\n" +
				"definition tokens, calls this week and the week before, and a verdict: used, rarely\n" +
				"used, unused, or learning (seen for less than a week). -text prints the same table; -json\n" +
				"prints it for scripts; -open=false only prints the page's address. Without a display, under\n" +
				"CI or with GASCOIGNE_SERVER=off the table is printed. Data: the daily per-tool stats\n" +
				"mcp-proxy records, kept a year (ledger stats_ttl).",
			examples: []string{"gascoigne mcp-review", "gascoigne mcp-review -text", "gascoigne mcp-review -json"},
			run:      cmdMCPReview},
		{name: "probe", args: "[flags]", group: groupSavings,
			summary: "check recorded runs for error lines and artifacts their output lost",
			about: "Compares stored originals with their optimized output: error lines, artifacts (paths,\n" +
				"URLs, hashes) and key=number pairs still readable, per family, and the runs that lost most.",
			examples: []string{"gascoigne probe", "gascoigne probe -since 7d -worst 20", "gascoigne probe -json"},
			run:      cmdProbe},
		{name: "prompts", args: "[flags] [dir]", group: groupSavings,
			summary: "list the instruction text an agent sends with every request, and its cost",
			about: "Reads the instruction files an agent loads for dir (CLAUDE.md chain with @imports, auto\n" +
				"memory, skill and subagent descriptions; codex: AGENTS.md), counts and prices them, and\n" +
				"reports large imports, lines sent twice and long descriptions.",
			examples:     []string{"GASCOIGNE_EXPERIMENTAL=prompts gascoigne prompts", "gascoigne prompts -agent codex -json"},
			run:          cmdPrompts,
			experimental: true},
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

		{name: "prose", args: "[install|status] [flags]", group: groupDev, experimental: true,
			summary: "set up the prose model: download model and ONNX Runtime, show whether it can run",
			about: "install downloads the model (274 MB) and the ONNX Runtime libraries for this platform into the\n" +
				"user cache dir, checking pinned SHA256 hashes, and prints them. status says whether the model is\n" +
				"installed. -name picks the model: kompress-v2-base, or ctxprune-small (563 MB, also trained on tool\n" +
				"output, logs and code; the config's optimizers.prose.model by default). The prose optimizer is on with this experiment and loads the model in the process that\n" +
				"optimizes: the background server keeps it loaded.\n" +
				"GPU: CUDA on Linux and Windows (needs the CUDA runtime and cuDNN 9), CoreML on macOS, else the CPU.\n" +
				"Experimental: enable with experimental: [prose] in the config or GASCOIGNE_EXPERIMENTAL=prose.\n" +
				"See docs/prose-model.md.",
			examples: []string{"gascoigne prose install", "gascoigne prose install -runtime cpu", "gascoigne prose install -name ctxprune-small", "gascoigne prose status"},
			run:      cmdProse},
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
	i := slices.IndexFunc(commandList, func(c command) bool { return c.name == name && available(c) })
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
	if i := slices.IndexFunc(commandList, func(c command) bool { return c.name == name }); i >= 0 {
		return fmt.Errorf("%q is experimental: turn it on with gascoigne setup -experimental, add it to experimental in the config, or set GASCOIGNE_EXPERIMENTAL=%s", name, name)
	}
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
		if !available(c) {
			continue
		}
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
			if c.group != g || !available(c) {
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
