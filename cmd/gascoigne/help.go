package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

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
func available(cmd command) bool { return !cmd.experimental || experimentalOn(cmd.name) }

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

// commandList is the command table, built on first use. It is assigned in init
// because the command funcs refer back to help, which a variable initializer
// cannot do without an initialization cycle.
var commandList func() []command

func init() {
	commandList = sync.OnceValue(func() []command {
		return slices.Concat(agentCommands(), optimizeCommands(), ledgerCommands(), insightCommands(), dashboardAndDevCommands())
	})
}

func lookup(name string) (command, bool) {
	i := slices.IndexFunc(commandList(), func(cmd command) bool { return cmd.name == name && available(cmd) })
	if i < 0 {
		return command{}, false
	}
	return commandList()[i], true
}

func cmdHelp(ctx context.Context, args []string) error {
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
	return c.run(ctx, []string{"-h"})
}

func unknownCommand(name string) error {
	if i := slices.IndexFunc(commandList(), func(cmd command) bool { return cmd.name == name }); i >= 0 {
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
	for _, c := range commandList() {
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
func distance(from, to string) int {
	prev := make([]int, len(to)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(from); i++ {
		cur := make([]int, len(to)+1)
		cur[0] = i
		for j := 1; j <= len(to); j++ {
			cost := 1
			if from[i-1] == to[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(to)]
}

// painterFor colors help on a terminal without NO_COLOR.
func painterFor(out io.Writer) painter {
	f, ok := out.(*os.File)
	if !ok {
		return painter{}
	}
	fi, err := f.Stat()
	return painter{err == nil && fi.Mode()&os.ModeCharDevice != 0 && os.Getenv("NO_COLOR") == ""}
}

func printOverview(out io.Writer) {
	p := painterFor(out)
	fmt.Fprintf(out, "%s %s shrinks what LLM agents read, keeping every original retrievable.\n\n", p.paint(bold, "gascoigne"), p.paint(dim, version.String()))
	fmt.Fprintf(out, "%s\n  gascoigne %s %s\n", p.paint(bold, "USAGE"), p.paint(cyan, "<command>"), p.paint(dim, "[flags]"))
	const width = 26
	for _, g := range groups {
		fmt.Fprintf(out, "\n%s\n", p.paint(bold, strings.ToUpper(g)))
		for _, c := range commandList() {
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
			fmt.Fprintf(out, "  %s%s  %s\n", styled, strings.Repeat(" ", max(width-len(plain), 0)), c.summary)
		}
	}
	fmt.Fprintf(out, "\n%s\n", p.paint(bold, "GET STARTED"))
	for _, e := range []string{
		"gascoigne install -g              # every agent, every project",
		"gascoigne exec -- go test ./...   # one command by hand",
		"gascoigne gain                    # where the savings come from",
		"gascoigne dashboard               # live view in the browser",
	} {
		fmt.Fprintf(out, "  %s\n", example(p, e))
	}
	fmt.Fprintf(out, "\nRun %s for a command's flags and examples. Docs: docs/README.md\n", p.paint(cyan, "gascoigne help <command>"))
}

// example dims a trailing "# comment".
func example(paint painter, text string) string {
	cmd, comment, ok := strings.Cut(text, "#")
	if !ok {
		return text
	}
	return cmd + paint.paint(dim, "#"+comment)
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

func printCommand(out io.Writer, name string, fs *flag.FlagSet) {
	p := painterFor(out)
	c, _ := lookup(strings.Fields(name)[0])
	fmt.Fprintf(out, "%s %s %s %s\n", p.paint(bold, "gascoigne"), p.paint(bold+";"+cyan, c.name), p.paint(dim, "—"), c.summary)
	fmt.Fprintf(out, "\n%s\n  %s\n", p.paint(bold, "USAGE"), strings.TrimSpace("gascoigne "+p.paint(cyan, c.name)+" "+p.paint(dim, c.args)))
	if c.about != "" {
		fmt.Fprintf(out, "\n%s\n", indent(c.about))
	}
	var own, shared []*flag.Flag
	fs.VisitAll(func(item *flag.Flag) {
		if slices.Contains(commonFlags, item.Name) {
			shared = append(shared, item)
			return
		}
		own = append(own, item)
	})
	printFlags(out, p, "FLAGS", own)
	if len(c.examples) > 0 {
		fmt.Fprintf(out, "\n%s\n", p.paint(bold, "EXAMPLES"))
		for _, e := range c.examples {
			fmt.Fprintf(out, "  %s\n", e)
		}
	}
	printFlags(out, p, "SHARED FLAGS", shared)
}

func printFlags(out io.Writer, paint painter, title string, flags []*flag.Flag) {
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
	fmt.Fprintf(out, "\n%s\n", paint.paint(bold, title))
	for _, r := range rows {
		pad := width - len(r.name)
		lead := "  " + paint.paint(cyan, r.name)
		if pad < 0 { // too long: description on the next line
			fmt.Fprintf(out, "%s\n", lead)
			lead, pad = "  ", width
		}
		lines := wrap(r.usage, 100-width-4)
		fmt.Fprintf(out, "%s%s  %s\n", lead, strings.Repeat(" ", pad), lines[0])
		for _, l := range lines[1:] {
			fmt.Fprintf(out, "  %s  %s\n", strings.Repeat(" ", width), l)
		}
	}
}

func indent(text string) string {
	return "  " + strings.ReplaceAll(text, "\n", "\n  ")
}

// wrap breaks s into lines of at most n bytes at spaces.
func wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		if line != "" && len(line)+1+len(word) > width {
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
