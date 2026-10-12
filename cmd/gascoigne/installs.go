package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// registrationVersion numbers what install writes into agent configs: hooks,
// matchers, guide text, rules. Bump it whenever that set changes, so installs
// made by an older gascoigne are re-registered (syncInstalls, gascoigne upgrade).
const registrationVersion = 2

// registrationNotes say what a registration version changed, for the
// question that offers the update.
var registrationNotes = map[int]string{
	2: "claude commands /gascoigne-stats and /gascoigne-doctor, and allow rules for gascoigne's own tools when chosen",
}

// An install is one agent registered for good (gascoigne install), recorded
// so a later gascoigne can bring its config up to date. Hooks call the binary
// by name, so what they do updates by itself; this is about what they are.
type install struct {
	Agent        string `json:"agent"`
	Global       bool   `json:"global,omitempty"`
	Dir          string `json:"dir,omitempty"` // the project, for a project install
	Exe          string `json:"exe"`           // how the config calls gascoigne
	Proxy        bool   `json:"proxy,omitempty"`
	Registration int    `json:"registration"`
}

func installsFile() string { return filepath.Join(stateDir(), "installs.json") }

// readInstalls are the recorded installs plus the legacy ones found in the
// agents' own config files (legacyInstalls).
func readInstalls() []install {
	var list []install
	if data, err := os.ReadFile(installsFile()); err == nil {
		json.Unmarshal(data, &list) // unreadable: nothing recorded
	}
	return append(list, legacyInstalls(list)...)
}

// legacyInstalls finds installs made before installs.json existed: the
// agent's config holds gascoigne's hook but nothing records it. They count as
// registration 0, so a gascoigne newer than the one that wrote them offers the
// update. Only the user-level configs and this project's are looked at.
func legacyInstalls(known []install) []install {
	exe, err := os.Executable()
	if err != nil {
		return nil
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	var out []install
	for _, name := range slices.Sorted(maps.Keys(agents)) {
		for _, e := range []install{
			{Agent: name, Global: true},
			{Agent: name, Dir: cwd},
		} {
			if !e.Global && (cwd == "" || cwd == home) {
				continue // the user-level config is the same file
			}
			if slices.ContainsFunc(known, e.same) {
				continue
			}
			if data, err := os.ReadFile(configOf(e)); err == nil && strings.Contains(string(data), "hook "+name) {
				e.Exe = installedName(exe)
				out = append(out, e)
			}
		}
	}
	return out
}

func writeInstalls(list []install) error {
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(list, "", "  ")
	tmp := installsFile() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, installsFile())
}

func (i install) same(o install) bool {
	return i.Agent == o.Agent && i.Global == o.Global && i.Dir == o.Dir
}

// recordInstall adds or replaces the entry for an agent in a scope.
func recordInstall(name string, global bool, exe string, proxy bool) error {
	e := install{Agent: name, Global: global, Exe: exe, Proxy: proxy, Registration: registrationVersion}
	if !global {
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		e.Dir = dir
	}
	list := slices.DeleteFunc(readInstalls(), e.same)
	return writeInstalls(append(list, e))
}

// forgetInstalls drops the entries of agents that unwrap removed.
func forgetInstalls(names []string) error {
	list := readInstalls()
	kept := slices.DeleteFunc(slices.Clone(list), func(e install) bool { return slices.Contains(names, e.Agent) })
	if len(kept) == len(list) {
		return nil
	}
	return writeInstalls(kept)
}

// syncInstalls registers again the user-scope installs older than
// registrationVersion, for agent hooks: only when the user chose autosync and
// $GASCOIGNE_AUTOSYNC is not off. Project files sit in repositories, so they
// are left to gascoigne upgrade, run in the project. It returns what it did,
// one line per install, and never fails the caller.
func syncInstalls() []string {
	if os.Getenv("GASCOIGNE_AUTOSYNC") == "off" || readPrefs().Autosync != "on" {
		return nil
	}
	stale := func(e install) bool { return e.Registration < registrationVersion && e.Global }
	if !slices.ContainsFunc(readInstalls(), stale) {
		return nil
	}
	unlock, ok := lockInstalls()
	if !ok {
		return nil // another process is syncing
	}
	defer unlock()
	list := readInstalls() // again, under the lock
	var out []string
	for i, e := range list {
		a, known := agents[e.Agent]
		if !stale(e) || !known {
			continue
		}
		if err := a.resync(e); err != nil {
			out = append(out, fmt.Sprintf("%s: could not update its gascoigne hooks: %v", e.Agent, err))
			continue
		}
		list[i].Registration = registrationVersion
		out = append(out, e.Agent+": updated its gascoigne hooks")
	}
	if err := writeInstalls(list); err != nil {
		out = append(out, "could not record the update: "+err.Error())
	}
	return out
}

// resync writes an install's config again: the agent's sync step when it has
// one (claude: settings only, not ~/.claude.json, which a running Claude
// Code rewrites), else its install; the LLM proxy again if it was chosen.
func (a agent) resync(e install) error {
	write := a.add
	if a.sync != nil {
		write = a.sync
	}
	if _, err := write(e.Exe, e.Global); err != nil {
		return err
	}
	if e.Proxy && a.addProxy != nil {
		_, err := a.addProxy(e.Global)
		return err
	}
	return nil
}

// lockInstalls takes installs.lock; a lock older than a minute is a crashed
// process's and is taken over.
func lockInstalls() (unlock func(), ok bool) {
	file := filepath.Join(stateDir(), "installs.lock")
	if err := os.MkdirAll(stateDir(), 0o700); err != nil {
		return nil, false
	}
	for range 2 {
		f, err := os.OpenFile(file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(file) }, true
		}
		if info, serr := os.Stat(file); !errors.Is(err, os.ErrExist) || serr != nil || time.Since(info.ModTime()) < time.Minute {
			return nil, false
		}
		os.Remove(file)
	}
	return nil, false
}

// syncNote runs the automatic update and words what it did for the agent,
// which reads its settings at start: new hooks work after a restart.
func syncNote() string {
	lines := syncInstalls()
	if len(lines) == 0 {
		return ""
	}
	return "gascoigne: " + strings.Join(lines, "; ") + ". Restart the agent to use new hooks."
}
