package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Other MCP servers reach agents through gascoigne mcp-proxy, so their tool
// results are optimized too: {"command": "npx", "args": ["-y", "srv"]}
// becomes {"command": "gascoigne", "args": ["mcp-proxy", "--", "npx", "-y", "srv"]}.
// The original command stays in args, so unwrap restores it exactly.

// proxyResult says what happened to an agent's other MCP servers.
type proxyResult struct {
	proxied []string
	direct  []string // "name (reason)": servers that cannot go through the proxy
}

func (r proxyResult) String() string {
	var parts []string
	if len(r.proxied) > 0 {
		parts = append(parts, "proxied MCP servers "+strings.Join(r.proxied, ", "))
	}
	if len(r.direct) > 0 {
		parts = append(parts, "left direct "+strings.Join(r.direct, ", "))
	}
	return strings.Join(parts, "; ")
}

// proxyCommand is how gascoigne starts a server: exe mcp-proxy -- cmd args...
func proxyCommand(exe, cmd string, args []string) (string, []string) {
	return exe, append([]string{"mcp-proxy", "--", cmd}, args...)
}

// proxiedArgs returns the original command of a proxied server.
func proxiedArgs(cmd string, args []string) ([]string, bool) {
	if !strings.Contains(cmd, "gascoigne") || len(args) < 3 || args[0] != "mcp-proxy" || args[1] != "--" {
		return nil, false
	}
	return args[2:], true
}

// proxyServers rewrites the stdio servers of an mcpServers object (claude,
// cursor) to start through the proxy. gascoigne itself and servers already
// proxied are left as they are; skip names servers not to touch.
func proxyServers(exe string, servers *object, skip func(name string) bool) proxyResult {
	var r proxyResult
	if servers == nil {
		return r
	}
	for _, name := range servers.keys {
		e, ok := servers.get(name).(*object)
		if !ok || name == "gascoigne" || skip != nil && skip(name) {
			continue
		}
		if reason := proxyEntry(exe, e); reason != "" {
			r.direct = append(r.direct, name+" ("+reason+")")
			continue
		}
		r.proxied = append(r.proxied, name)
	}
	return r
}

// proxyEntry rewrites one server entry in place; a reason when it cannot.
func proxyEntry(exe string, entry *object) string {
	typ, _ := entry.get("type").(string)
	if entry.get("url") != nil && (typ == "http" || typ == "sse" || typ == "") {
		return proxyRemoteEntry(exe, entry, typ)
	}
	if typ != "" && typ != "stdio" || entry.get("url") != nil || entry.get("httpUrl") != nil {
		return "remote: only stdio servers and typed http/sse servers can be proxied"
	}
	cmd, _ := entry.get("command").(string)
	if cmd == "" {
		return "no command"
	}
	args, ok := stringsOf(entry.get("args"))
	if !ok {
		return "args are not strings"
	}
	if _, done := proxiedArgs(cmd, args); done {
		return ""
	}
	if _, _, _, done := remoteOf(cmd, args); done {
		return ""
	}
	newCmd, newArgs := proxyCommand(exe, cmd, args)
	entry.set("command", newCmd)
	entry.set("args", anys(newArgs))
	return ""
}

// proxyRemoteEntry turns a typed http/sse entry into a stdio one running the
// proxy with -url; unproxyServers turns it back.
func proxyRemoteEntry(exe string, entry *object, typ string) string {
	url, _ := entry.get("url").(string)
	var headers []string
	if h := entry.get("headers"); h != nil {
		ho, ok := h.(*object)
		if !ok {
			return "headers are not an object"
		}
		for _, k := range ho.keys {
			v, ok := ho.get(k).(string)
			if !ok {
				return "headers are not strings"
			}
			headers = append(headers, k+": "+v)
		}
	}
	if entry.get("oauth") != nil || entry.get("headersHelper") != nil {
		return "remote: OAuth and header helpers are not supported"
	}
	if typ == "" && (entry.get("command") != nil || entry.get("httpUrl") != nil) {
		return "remote: both url and command or httpUrl"
	}
	for _, k := range []string{"type", "url", "headers"} {
		entry.del(k)
	}
	entry.set("command", exe)
	entry.set("args", anys(remoteProxyArgs(url, typ, headers)))
	return ""
}

// remoteProxyArgs is how gascoigne starts a remote server: mcp-proxy -url U [-sse|-auto] [-header "K: V"]...
func remoteProxyArgs(url, typ string, headers []string) []string {
	args := []string{"mcp-proxy", "-url", url}
	switch typ {
	case "sse":
		args = append(args, "-sse")
	case "":
		args = append(args, "-auto")
	}
	for _, h := range headers {
		args = append(args, "-header", h)
	}
	return args
}

// remoteOf parses remoteProxyArgs back; false for any other command.
func remoteOf(cmd string, args []string) (url, typ string, headers []string, ok bool) {
	if !strings.Contains(cmd, "gascoigne") || len(args) < 3 || args[0] != "mcp-proxy" || args[1] != "-url" {
		return
	}
	url, rest := args[2], args[3:]
	typ = "http"
	for len(rest) > 0 {
		switch {
		case rest[0] == "-sse":
			typ, rest = "sse", rest[1:]
		case rest[0] == "-auto":
			typ, rest = "", rest[1:]
		case rest[0] == "-header" && len(rest) > 1:
			headers, rest = append(headers, rest[1]), rest[2:]
		default:
			return "", "", nil, false
		}
	}
	return url, typ, headers, true
}

// unproxyServers restores proxied servers of an mcpServers object.
func unproxyServers(servers *object) []string {
	var names []string
	if servers == nil {
		return nil
	}
	for _, name := range servers.keys {
		e, ok := servers.get(name).(*object)
		if !ok {
			continue
		}
		cmd, _ := e.get("command").(string)
		args, _ := stringsOf(e.get("args"))
		if url, typ, headers, ok := remoteOf(cmd, args); ok {
			unproxyRemoteEntry(e, url, typ, headers)
			names = append(names, name)
			continue
		}
		orig, ok := proxiedArgs(cmd, args)
		if !ok {
			continue
		}
		e.set("command", orig[0])
		if len(orig) == 1 {
			e.del("args")
		} else {
			e.set("args", anys(orig[1:]))
		}
		names = append(names, name)
	}
	return names
}

func unproxyRemoteEntry(entry *object, url, typ string, headers []string) {
	entry.del("command")
	entry.del("args")
	if typ != "" {
		entry.set("type", typ)
	}
	entry.set("url", url)
	if len(headers) == 0 {
		return
	}
	h := &object{vals: map[string]any{}}
	for _, kv := range headers {
		k, v, _ := strings.Cut(kv, ":")
		h.set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	entry.set("headers", h)
}

func stringsOf(value any) ([]string, bool) {
	if value == nil {
		return nil, true
	}
	list, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, len(list))
	for i, x := range list {
		s, ok := x.(string)
		if !ok {
			return nil, false
		}
		out[i] = s
	}
	return out, true
}

func anys(items []string) []any {
	out := make([]any, len(items))
	for i, x := range items {
		out[i] = x
	}
	return out
}

// claudeJSON is Claude Code's state file, home of user and local MCP servers.
func claudeJSON() string {
	home := home()
	return filepath.Join(home, ".claude.json")
}

func readJSONObject(file string) *object {
	data, err := os.ReadFile(file)
	if err != nil {
		return nil
	}
	v, err := parseJSON(data)
	if err != nil {
		return nil
	}
	o, _ := v.(*object)
	return o
}

// claudeProxies are the stdio servers Claude would start in this directory,
// rewritten to go through the proxy, for wrap's --mcp-config: a server
// given there replaces the same name from settings for the run. Project
// (.mcp.json) servers count only once approved, so wrap never starts one
// the user has not allowed.
func claudeProxies(exe string) (map[string]any, proxyResult) {
	cwd, _ := os.Getwd()
	state := readJSONObject(claudeJSON())
	local := obj(path(state, "projects", cwd))
	disabled := append(stringList(path(local, "disabledMcpServers")), stringList(path(local, "disabledMcpjsonServers"))...)

	servers := map[string]*object{}
	order := []string{}
	put := func(from *object, ok func(string) bool) {
		if from == nil {
			return
		}
		for _, name := range from.keys {
			e, isObj := from.get(name).(*object)
			if !isObj || !ok(name) || slices.Contains(disabled, name) {
				continue
			}
			if _, seen := servers[name]; !seen {
				order = append(order, name)
			}
			servers[name] = e
		}
	}
	all := func(string) bool { return true }
	// Lowest precedence first: user, project, local.
	put(obj(path(state, "mcpServers")), all)
	put(obj(path(readJSONObject(".mcp.json"), "mcpServers")), func(name string) bool { return projectApproved(local, name) })
	put(obj(path(local, "mcpServers")), all)

	picked := &object{vals: map[string]any{}}
	for _, name := range order {
		picked.set(name, servers[name])
	}
	r := proxyServers(exe, picked, nil)
	out := map[string]any{}
	for _, name := range r.proxied {
		out[name] = toMaps(picked.get(name))
	}
	return out, r
}

// projectApproved: the user allowed this .mcp.json server, in Claude's state
// or any settings file.
func projectApproved(local *object, name string) bool {
	if slices.Contains(stringList(path(local, "disabledMcpjsonServers")), name) {
		return false
	}
	if slices.Contains(stringList(path(local, "enabledMcpjsonServers")), name) {
		return true
	}
	for _, f := range configFiles(".claude/settings.json", ".claude/settings.json", ".claude/settings.local.json") {
		s := readJSONObject(f)
		if path(s, "enableAllProjectMcpServers") == true || slices.Contains(stringList(path(s, "enabledMcpjsonServers")), name) {
			return true
		}
	}
	return false
}

func obj(value any) *object {
	o, _ := value.(*object)
	return o
}

func stringList(value any) []string {
	s, _ := stringsOf(value)
	return s
}

// toMaps turns parsed JSON (ordered objects) back into maps for encoding.
func toMaps(value any) any {
	switch x := value.(type) {
	case *object:
		m := make(map[string]any, len(x.keys))
		for _, k := range x.keys {
			m[k] = toMaps(x.vals[k])
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = toMaps(e)
		}
		return out
	case json.Number:
		return x
	}
	return value
}

// codexServer is one [mcp_servers.NAME] table of a codex config.
type codexServer struct {
	name   string
	cmd    string
	args   []string
	remote bool
}

// codexServerName is NAME of a "[mcp_servers.NAME]" header, "" for others
// (subtables like [mcp_servers.NAME.env] included).
func codexServerName(header string) string {
	h := strings.TrimSpace(header)
	if !strings.HasPrefix(h, "[") || strings.HasPrefix(h, "[[") {
		return ""
	}
	key, ok := strings.CutPrefix(strings.Trim(h, "[] "), "mcp_servers.")
	if !ok {
		return ""
	}
	if name, err := tomlString(key); err == nil && len(name)+2 == len(key) {
		return name // quoted: [mcp_servers."my.server"]
	}
	if strings.ContainsAny(key, `."' `) {
		return ""
	}
	return key
}

func codexServerOf(name, body string) codexServer {
	s := codexServer{name: name}
	if _, _, v, ok := tomlKey(body, "url"); ok && v != "" {
		s.remote = true
	}
	if _, _, v, ok := tomlKey(body, "command"); ok {
		s.cmd, _ = tomlString(v)
	}
	if _, _, v, ok := tomlKey(body, "args"); ok {
		s.args, _ = tomlStrings(v)
	}
	return s
}

// codexServers lists the MCP servers of a codex config.
func codexServers(text string) []codexServer {
	var out []codexServer
	for _, sec := range tomlSections(text) {
		if name := codexServerName(sec.header); name != "" && name != "gascoigne" {
			out = append(out, codexServerOf(name, sec.body))
		}
	}
	return out
}

// codexProxyArgs are wrap's -c overrides that start codex's stdio servers
// through the proxy for one run.
func codexProxyArgs(exe string, servers []codexServer) ([]string, proxyResult) {
	var args []string
	var r proxyResult
	for _, s := range servers {
		switch {
		case s.remote:
			r.direct = append(r.direct, s.name+" (remote: only stdio servers can be proxied)")
			continue
		case s.cmd == "":
			r.direct = append(r.direct, s.name+" (no command)")
			continue
		case strings.Trim(s.name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-") != "":
			r.direct = append(r.direct, s.name+" (name needs quoting in -c)")
			continue
		}
		if _, done := proxiedArgs(s.cmd, s.args); done {
			continue
		}
		cmd, cargs := proxyCommand(exe, s.cmd, s.args)
		args = append(args, "-c", "mcp_servers."+s.name+".command="+tomlQuote(cmd), "-c", "mcp_servers."+s.name+".args="+tomlArray(cargs))
		r.proxied = append(r.proxied, s.name)
	}
	return args, r
}
