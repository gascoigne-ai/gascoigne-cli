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

// A deny rule looks the same whoever wrote it, so install records the rules
// it added per settings file, and unwrap removes only those.
func deniesFile() string { return filepath.Join(stateDir(), "denies.json") }

func readDenies() map[string][]string {
	m := map[string][]string{}
	if data, err := os.ReadFile(deniesFile()); err == nil {
		json.Unmarshal(data, &m) // unreadable: nothing recorded
	}
	return m
}

func writeDenies(m map[string][]string) error {
	if err := os.MkdirAll(filepath.Dir(deniesFile()), 0o755); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(m, "", "  ")
	return os.WriteFile(deniesFile(), data, 0o644)
}

// denyList returns the permissions.deny list of a settings object.
func denyList(root *object) []string {
	list, _ := stringsOf(path(root, "permissions", "deny"))
	return list
}

// addDenies adds the rules missing from root's permissions.deny and returns
// them; recordDenies remembers them for unwrap.
func addDenies(root *object, rules []string) []string {
	have := denyList(root)
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
	perms.set("deny", anys(append(have, added...)))
	return added
}

func recordDenies(file string, added []string) error {
	if len(added) == 0 {
		return nil
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	m := readDenies()
	m[abs] = slices.Compact(slices.Sorted(slices.Values(append(m[abs], added...))))
	return writeDenies(m)
}

// removeDenies takes out of root the rules install recorded for file.
func removeDenies(file string, root *object) []string {
	abs, err := filepath.Abs(file)
	if err != nil {
		return nil
	}
	m := readDenies()
	ours := m[abs]
	if len(ours) == 0 {
		return nil
	}
	have := denyList(root)
	kept := slices.DeleteFunc(slices.Clone(have), func(r string) bool { return slices.Contains(ours, r) })
	delete(m, abs)
	if err := writeDenies(m); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne:", err)
	}
	if len(kept) == len(have) {
		return nil
	}
	if perms := obj(root.get("permissions")); perms != nil {
		perms.set("deny", anys(kept))
		pruneEmpty(perms, "deny")
		pruneEmpty(root, "permissions")
	}
	return []string{"deny " + strings.Join(ours, ", ")}
}
