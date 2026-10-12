package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
)

// Where an MCP server in the review is configured. mcp-proxy knows a server by
// the name it reports and a hash of its command line; the agent knows it by
// its own key. The hash is in both places: the entry that runs
// "gascoigne mcp-proxy -- <command>" holds the command. The review shows the
// entry and how to switch the server off; gascoigne does not write other
// programs' settings unasked (decisions.md), so that step stays the user's.

// mcpConfigEntry is one config entry that starts a proxied server.
type mcpConfigEntry struct {
	Agent  string `json:"agent"`
	Key    string `json:"key"`    // the agent's name for the server
	File   string `json:"file"`   // the config file holding it
	Scope  string `json:"scope"`  // user, local or project
	Remove string `json:"remove"` // how to switch it off
}

// proxiedConfigs finds the mcp-proxy entries of the agents' config files by the
// hash of the command they run: Claude's user and local servers (~/.claude.json),
// project .mcp.json files and Cursor's mcp.json under home and dirs.
func proxiedConfigs(home string, dirs []string) map[string][]mcpConfigEntry {
	found := map[string][]mcpConfigEntry{}
	add := func(servers map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}, agent, file, scope, dir string) {
		for key, e := range servers {
			orig, ok := proxiedArgs(e.Command, e.Args)
			if url, _, _, remote := remoteOf(e.Command, e.Args); remote {
				orig, ok = []string{url}, true // the receiver hashes a remote server's url
			}
			if !ok || len(orig) == 0 {
				continue
			}
			hash := mcpproxyreceiver.CommandHash(orig)
			found[hash] = append(found[hash], mcpConfigEntry{Agent: agent, Key: key, File: file, Scope: scope, Remove: removeCommand(agent, key, file, scope, dir)})
		}
	}
	type servers = map[string]struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	var cj struct {
		MCPServers servers `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers servers `json:"mcpServers"`
		} `json:"projects"`
	}
	if readJSONFile(filepath.Join(home, ".claude.json"), &cj) {
		add(cj.MCPServers, "claude", filepath.Join(home, ".claude.json"), "user", "")
		for dir, p := range cj.Projects {
			add(p.MCPServers, "claude", filepath.Join(home, ".claude.json"), "local", dir)
		}
	}
	files := []struct{ agent, path, scope, dir string }{{"cursor", filepath.Join(home, ".cursor", "mcp.json"), "user", ""}}
	for _, dir := range slices.Compact(slices.Sorted(slices.Values(dirs))) {
		files = append(files,
			struct{ agent, path, scope, dir string }{"claude", filepath.Join(dir, ".mcp.json"), "project", dir},
			struct{ agent, path, scope, dir string }{"cursor", filepath.Join(dir, ".cursor", "mcp.json"), "project", dir})
	}
	for _, f := range files {
		var pj struct {
			MCPServers servers `json:"mcpServers"`
		}
		if readJSONFile(f.path, &pj) {
			add(pj.MCPServers, f.agent, f.path, f.scope, f.dir)
		}
	}
	return found
}

// removeCommand says how to switch one server off, in the agent's own terms.
func removeCommand(agent, key, file, scope, dir string) string {
	switch {
	case agent == "claude" && scope == "user":
		return "claude mcp remove " + shellQuote(key) + " --scope user"
	case agent == "claude":
		return "cd " + shellQuote(dir) + " && claude mcp remove " + shellQuote(key) + " --scope " + scope
	}
	return "delete the " + shellQuote(key) + " entry from " + file
}

// readJSONFile reads path into v; false when it is missing or not JSON.
func readJSONFile(path string, v any) bool {
	data, err := os.ReadFile(path)
	return err == nil && json.Unmarshal(data, v) == nil
}

// projectDirs are the places a project-level MCP config can be: the projects
// gascoigne was installed in, and the current directory.
func projectDirs() []string {
	var dirs []string
	for _, in := range readInstalls() {
		if in.Dir != "" {
			dirs = append(dirs, in.Dir)
		}
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd)
	}
	return dirs
}
