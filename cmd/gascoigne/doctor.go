package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/version"
)

// A finding is one line of the doctor's report. fix, when set, repairs it
// (gascoigne upgrade runs it).
type finding struct {
	level string // ok, warn or problem
	text  string
	fix   func() error
}

// cmdDoctor reports whether gascoigne is set up the way its records say.
// gascoigne upgrade repairs what it finds.
func cmdDoctor(_ context.Context, args []string) error {
	if err := newFlags("doctor").Parse(args); err != nil {
		return err
	}
	problems := 0
	for _, f := range diagnose() {
		fmt.Printf("%-7s %s\n", f.level, f.text)
		if f.level == "problem" {
			problems++
		}
	}
	if problems > 0 {
		return fmt.Errorf("%d problem(s); gascoigne upgrade repairs them", problems)
	}
	return nil
}

// diagnose checks the binary, the background server, the saved answers and
// every recorded install.
func diagnose() []finding {
	var out []finding
	add := func(level, format string, args ...any) {
		out = append(out, finding{level: level, text: fmt.Sprintf(format, args...)})
	}

	exe, _ := os.Executable()
	if p, err := exec.LookPath("gascoigne"); err == nil && sameFile(p, exe) {
		add("ok", "gascoigne %s is on PATH", version.String())
	} else {
		add("warn", "gascoigne %s is not on PATH, so agent configs call the full path %s (project files cannot be shared)", version.String(), exe)
	}
	switch info, err := health(serverAddr()); {
	case serverOff():
		add("warn", "background server is switched off (GASCOIGNE_SERVER=off): no dashboard, no LLM proxy")
	case err != nil:
		add("ok", "background server: stopped; it starts by itself with the next command or agent prompt")
	default:
		add("ok", "background server: running at http://%s (pid %d)", info.Addr, info.PID)
	}

	p := readPrefs()
	add("ok", "usage text for agents: %s", guideLevel())
	switch r := readRelease(); {
	case !releaseCheckOn():
		add("ok", "release notice: off")
	case r.newer():
		add("warn", "gascoigne %s is out (you run %s): %s. After installing it, run gascoigne upgrade.", r.ID, version.Number, r.URL)
	default:
		add("ok", "release notice: on, every 15 min (%s); latest known: %s", releaseBase(), cmp.Or(r.ID, "not checked yet"))
	}
	var waiting []string
	for _, q := range pendingDecisions(p) {
		if !strings.HasPrefix(q.ID, integratePrefix) { // new agents are listed below
			waiting = append(waiting, q.Title)
		}
	}
	if len(waiting) > 0 {
		add("warn", "choices waiting for you: %s. Answer them with: gascoigne setup", strings.Join(waiting, "; "))
	}

	installs := readInstalls()
	for _, e := range installs {
		out = append(out, checkInstall(e))
	}
	for _, name := range foundAgents() {
		if !slices.ContainsFunc(installs, func(entry install) bool { return entry.Agent == name }) {
			add("warn", "%s: found on this machine, but gascoigne is not connected to it yet (connect it for all projects: gascoigne install -g %s)", name, name)
		}
	}
	if len(installs) == 0 && len(foundAgents()) == 0 {
		add("warn", "no coding agent found on this machine, and none connected to gascoigne")
	}
	return out
}

// checkInstall looks at one recorded install: its config file still holds
// the hook, and the registration is current.
func checkInstall(entry install) finding {
	scope := "all projects"
	if !entry.Global {
		scope = entry.Dir
	}
	label := entry.Agent + " (" + scope + ")"
	file := configOf(entry)
	data, err := os.ReadFile(file)
	lost := err != nil || !strings.Contains(string(data), "hook "+entry.Agent)
	stale := entry.Registration < registrationVersion
	f := finding{level: "problem"}
	switch {
	case lost:
		f.text = fmt.Sprintf("%s: gascoigne's hook is missing from %s", label, display(file))
	case stale:
		f.text = fmt.Sprintf("%s: set up by an older gascoigne, so newer hooks and rules are missing", label)
	default:
		return finding{level: "ok", text: label + ": connected"}
	}
	cwd, _ := os.Getwd()
	if !entry.Global && entry.Dir != cwd {
		f.text += ". Fix it by running gascoigne install " + entry.Agent + " in " + entry.Dir
		return f
	}
	f.text += ". Fix it with: gascoigne upgrade"
	f.fix = func() error {
		a, ok := agents[entry.Agent]
		if !ok {
			return fmt.Errorf("unknown agent %s", entry.Agent)
		}
		if err := a.resync(entry); err != nil {
			return err
		}
		return recordInstall(entry.Agent, entry.Global, entry.Exe, entry.Proxy)
	}
	return f
}

// configOf is the file whose gascoigne hook shows an install is in place.
func configOf(entry install) string {
	home := home()
	pick := func(user, project string) string {
		if entry.Global {
			return filepath.Join(home, user)
		}
		return filepath.Join(entry.Dir, project)
	}
	switch entry.Agent {
	case "codex":
		if entry.Global {
			return filepath.Join(codexHome(), "config.toml")
		}
		return filepath.Join(entry.Dir, ".codex", "config.toml")
	case "cursor":
		return pick(".cursor/hooks.json", ".cursor/hooks.json")
	case "gemini":
		return pick(".gemini/settings.json", ".gemini/settings.json")
	case "opencode":
		if entry.Global {
			return filepath.Join(opencodeHome(), "plugins", "gascoigne.ts")
		}
		return filepath.Join(entry.Dir, ".opencode", "plugins", "gascoigne.ts")
	case "pi":
		return pick(".pi/agent/extensions/gascoigne.ts", ".pi/extensions/gascoigne.ts")
	}
	return pick(".claude/settings.json", ".claude/settings.json")
}
