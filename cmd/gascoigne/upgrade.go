package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/version"
)

// cmdUpgrade gathers what a new gascoigne needs done in one place, so there
// is one command to remember after installing a new version:
//  1. hooks, rules and commands of recorded installs: stale or lost ones are
//     written again (user-level, and this project's);
//  2. the user's config.yaml: moved onto this version's template, settings kept;
//  3. choices still waiting for the user: asked in the browser.
//
// -n only lists what would change.
func cmdUpgrade(_ context.Context, args []string) error {
	fset := newFlags("upgrade")
	dry := fset.Bool("n", false, "only list what would change")
	if err := fset.Parse(args); err != nil {
		return err
	}
	did := upgradeHooks(*dry)
	if upgradeConfigFile(*dry) {
		did = true
	}
	if r := readRelease(); releaseCheckOn() && r.newer() {
		did = true
		fmt.Printf("release: gascoigne %s is out (you run %s): %s\n  Install it, then run gascoigne upgrade again.\n", r.ID, version.Number, r.URL)
	}
	if open := pendingDecisions(readPrefs()); len(open) > 0 {
		did = true
		if *dry {
			fmt.Println("choices: waiting for you; gascoigne setup asks")
		} else {
			offerSetup(os.Stdout)
		}
	}
	if !did {
		fmt.Println("everything is up to date")
	}
	return nil
}

// upgradeHooks writes stale or lost installs again; it reports whether there
// was anything to do.
func upgradeHooks(dry bool) bool {
	did := false
	for _, e := range readInstalls() {
		f := checkInstall(e)
		if f.level == "ok" {
			continue
		}
		did = true
		switch {
		case f.fix == nil:
			fmt.Println("hooks:", f.text)
		case dry:
			fmt.Println("hooks: would update:", f.text)
		default:
			if err := f.fix(); err != nil {
				fmt.Printf("hooks: %s: could not update: %v\n", e.Agent, err)
				continue
			}
			fmt.Printf("hooks: updated %s (%s); restart the agent to use new hooks\n", e.Agent, installScope(e))
		}
	}
	return did
}

func installScope(e install) string {
	if e.Global {
		return "all projects"
	}
	return e.Dir
}

// upgradeConfigFile moves the user's own config.yaml onto this version's
// template (the same as gascoigne config -upgrade); a config given with
// -config or $GASCOIGNE_CONFIG is not touched.
func upgradeConfigFile(dry bool) bool {
	path, own := configPath("")
	if !own || path == "" || !fileExists(path) {
		return false
	}
	if dry {
		old, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		out, dropped, err := upgradeConfig(old)
		if err != nil || bytes.Equal(out, old) {
			return false
		}
		fmt.Printf("config: would upgrade %s%s\n", path, droppedNote(dropped))
		return true
	}
	dropped, changed, err := writeUpgrade(path)
	if err != nil {
		fmt.Println("config:", err)
		return true
	}
	if !changed {
		return false
	}
	fmt.Printf("config: upgraded %s, your settings kept (previous file: %s.bak)%s\n", path, path, droppedNote(dropped))
	return true
}

func droppedNote(dropped []string) string {
	if len(dropped) == 0 {
		return ""
	}
	return "; dropped settings this version does not have: " + strings.Join(dropped, ", ")
}
