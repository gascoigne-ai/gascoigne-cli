package main

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/internal/mcptool"
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
	var cj struct {
		MCPServers mcpServerSet `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers mcpServerSet `json:"mcpServers"`
		} `json:"projects"`
	}
	claudeJSON := filepath.Join(home, ".claude.json")
	if readJSONFile(claudeJSON, &cj) {
		addProxied(found, cj.MCPServers, "claude", claudeJSON, "user", "")
		for dir, p := range cj.Projects {
			addProxied(found, p.MCPServers, "claude", claudeJSON, "local", dir)
		}
	}
	for _, f := range configFilesToScan(home, dirs) {
		var pj struct {
			MCPServers mcpServerSet `json:"mcpServers"`
		}
		if readJSONFile(f.path, &pj) {
			addProxied(found, pj.MCPServers, f.agent, f.path, f.scope, f.dir)
		}
	}
	return found
}

// mcpServerSet are the servers of an agent's config file.
type mcpServerSet = map[string]struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// addProxied adds to found, by command hash, the servers that run mcp-proxy.
func addProxied(found map[string][]mcpConfigEntry, servers mcpServerSet, agent, file, scope, dir string) {
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

// scanFile is a config file that may hold servers.
type scanFile struct{ agent, path, scope, dir string }

// configFilesToScan are Cursor's user file and each directory's project files.
func configFilesToScan(home string, dirs []string) []scanFile {
	files := []scanFile{{"cursor", filepath.Join(home, ".cursor", "mcp.json"), "user", ""}}
	for _, dir := range slices.Compact(slices.Sorted(slices.Values(dirs))) {
		files = append(files,
			scanFile{"claude", filepath.Join(dir, ".mcp.json"), "project", dir},
			scanFile{"cursor", filepath.Join(dir, ".cursor", "mcp.json"), "project", dir})
	}
	return files
}

// removeCommand says how to switch one server off, in the agent's own terms.
func removeCommand(agent, key, file, scope, dir string) string {
	switch {
	case agent == "claude" && scope == "user":
		return "claude mcp remove " + mcptool.ShellQuote(key) + " --scope user"
	case agent == "claude":
		return "cd " + mcptool.ShellQuote(dir) + " && claude mcp remove " + mcptool.ShellQuote(key) + " --scope " + scope
	}
	return "delete the " + mcptool.ShellQuote(key) + " entry from " + file
}

// readJSONFile reads path into target; false when it is missing or not JSON.
// A file that exists but is not JSON is logged at debug level.
func readJSONFile(path string, target any) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	if err := json.Unmarshal(data, target); err != nil {
		slog.Debug("not JSON, ignored", "file", filepath.Base(path), "err", err)
		return false
	}
	return true
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
