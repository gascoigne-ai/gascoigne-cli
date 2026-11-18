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
type findingLevel string

const (
	levelOK      findingLevel = "ok"
	levelWarn    findingLevel = "warn"
	levelProblem findingLevel = "problem"
)

type finding struct {
	level findingLevel
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
		if f.level == levelProblem {
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
	saved := readPrefs()
	out := []finding{pathFinding(), serverFinding()}
	out = append(out, findf(levelOK, "usage text for agents: %s", guideLevel()), releaseFinding())
	if w := waitingFinding(saved); w != nil {
		out = append(out, *w)
	}
	return append(out, installFindings()...)
}

// findf is a finding with a formatted text.
func findf(level findingLevel, format string, args ...any) finding {
	return finding{level: level, text: fmt.Sprintf(format, args...)}
}

// pathFinding says whether this binary is the gascoigne on PATH.
func pathFinding() finding {
	exe, _ := os.Executable()
	if p, err := exec.LookPath("gascoigne"); err == nil && sameFile(p, exe) {
		return findf(levelOK, "gascoigne %s is on PATH", version.String())
	}
	return findf(levelWarn, "gascoigne %s is not on PATH, so agent configs call the full path %s (project files cannot be shared)", version.String(), exe)
}

// serverFinding says whether the background server runs.
func serverFinding() finding {
	switch info, err := health(serverAddr()); {
	case serverOff():
		return findf(levelWarn, "background server is switched off (GASCOIGNE_SERVER=off): no dashboard, no LLM proxy")
	case err != nil:
		return findf(levelOK, "background server: stopped; it starts by itself with the next command or agent prompt")
	default:
		return findf(levelOK, "background server: running at http://%s (pid %d)", info.Addr, info.PID)
	}
}

// releaseFinding says whether the release notice is on and a newer release is out.
func releaseFinding() finding {
	switch r := readRelease(); {
	case !releaseCheckOn():
		return findf(levelOK, "release notice: off")
	case r.newer():
		return findf(levelWarn, "gascoigne %s is out (you run %s): %s. After installing it, run gascoigne upgrade.", r.ID, version.Number, r.URL)
	default:
		return findf(levelOK, "release notice: on, every 15 min (%s); latest known: %s", releaseBase(), cmp.Or(r.ID, "not checked yet"))
	}
}

// waitingFinding lists the setup choices still open, nil without any.
func waitingFinding(saved prefs) *finding {
	var waiting []string
	for _, q := range pendingDecisions(saved) {
		if !strings.HasPrefix(q.ID, integratePrefix) { // new agents are listed below
			waiting = append(waiting, q.Title)
		}
	}
	if len(waiting) == 0 {
		return nil
	}
	f := findf(levelWarn, "choices waiting for you: %s. Answer them with: gascoigne setup", strings.Join(waiting, "; "))
	return &f
}

// installFindings checks each recorded install and names agents found but not
// connected.
func installFindings() []finding {
	var out []finding
	installs := readInstalls()
	for _, e := range installs {
		out = append(out, checkInstall(e))
	}
	for _, name := range foundAgents() {
		if !slices.ContainsFunc(installs, func(entry install) bool { return entry.Agent == name }) {
			out = append(out, findf(levelWarn, "%s: found on this machine, but gascoigne is not connected to it yet (connect it for all projects: gascoigne install -g %s)", name, name))
		}
	}
	if len(installs) == 0 && len(foundAgents()) == 0 {
		out = append(out, findf(levelWarn, "no coding agent found on this machine, and none connected to gascoigne"))
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
	f := finding{level: levelProblem}
	switch {
	case lost:
		f.text = fmt.Sprintf("%s: gascoigne's hook is missing from %s", label, display(file))
	case stale:
		f.text = fmt.Sprintf("%s: set up by an older gascoigne, so newer hooks and rules are missing", label)
	default:
		return finding{level: levelOK, text: label + ": connected"}
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
