package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// claudeDenies are the built-in tools whose results no hook can shrink well:
// searches then run as rg and find through Bash, pages through gascoigne's
// fetch. Read stays (Edit needs it), WebSearch stays (no replacement).
var claudeDenies = []string{"Grep", "Glob", "WebFetch"}

// Allow levels: which of gascoigne's own MCP tools run without an approval
// prompt. Fetch is apart from the rest: it reaches the network.
var (
	allowLevels    = []string{"none", "local", "all"}
	claudeAllowsLo = []string{"mcp__gascoigne__retrieve", "mcp__gascoigne__show", "mcp__gascoigne__stats", "mcp__gascoigne__compress"}
)

// claudeAllows are the permissions.allow rules for a level.
func claudeAllows(level string) []string {
	switch level {
	case "local":
		return claudeAllowsLo
	case "all":
		return append(slices.Clone(claudeAllowsLo), "mcp__gascoigne__fetch")
	}
	return nil
}

// A rule looks the same whoever wrote it, so install records the rules it
// added per settings file and kind, and unwrap removes only those.
type ruleKind struct {
	list string // permissions.<list> in the settings file
	file string // state file with the recorded rules
}

var (
	denyRules  = ruleKind{"deny", "denies.json"}
	allowRules = ruleKind{"allow", "allows.json"}
)

func (k ruleKind) stateFile() string { return filepath.Join(stateDir(), k.file) }

func (k ruleKind) read() map[string][]string {
	m := map[string][]string{}
	if data, err := os.ReadFile(k.stateFile()); err == nil {
		json.Unmarshal(data, &m) // unreadable: nothing recorded
	}
	return m
}

func (k ruleKind) write(m map[string][]string) error {
	if err := os.MkdirAll(stateDir(), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(k.stateFile(), data, 0o644)
}

// rules returns the permissions list of a settings object.
func (k ruleKind) rules(root *object) []string {
	list, _ := stringsOf(path(root, "permissions", k.list))
	return list
}

// add adds the rules missing from root's list and returns them; record
// remembers them for unwrap.
func (k ruleKind) add(root *object, rules []string) []string {
	have := k.rules(root)
	var added []string
	for _, r := range rules {
		if !slices.Contains(have, r) {
			added = append(added, r)
		}
	}
	if len(added) == 0 {
		return nil
	}
	perms := obj(root.get("permissions"))
	if perms == nil {
		// First, so removing and re-adding gascoigne's hooks on a later
		// install leaves the key order, and the file, as it is.
		perms = toObject(map[string]any{})
		root.set("permissions", perms)
		root.keys = append([]string{"permissions"}, slices.DeleteFunc(root.keys, func(k string) bool { return k == "permissions" })...)
	}
	perms.set(k.list, anys(append(have, added...)))
	return added
}

func (k ruleKind) record(file string, added []string) error {
	if len(added) == 0 {
		return nil
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	m := k.read()
	m[abs] = slices.Compact(slices.Sorted(slices.Values(append(m[abs], added...))))
	return k.write(m)
}

// remove takes out of root the rules install recorded for file.
func (k ruleKind) remove(file string, root *object) []string {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil
	}
	m := k.read()
	ours := m[abs]
	if len(ours) == 0 {
		return nil
	}
	have := k.rules(root)
	kept := slices.DeleteFunc(slices.Clone(have), func(r string) bool { return slices.Contains(ours, r) })
	delete(m, abs)
	if err := k.write(m); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
	}
	if len(kept) == len(have) {
		return nil
	}
	if perms := obj(root.get("permissions")); perms != nil {
		perms.set(k.list, anys(kept))
		pruneEmpty(perms, k.list)
		pruneEmpty(root, "permissions")
	}
	return []string{k.list + " " + strings.Join(ours, ", ")}
}

// removeRules takes out every recorded rule of both kinds.
func removeRules(file string, root *object) []string {
	return append(denyRules.remove(file, root), allowRules.remove(file, root)...)
}
