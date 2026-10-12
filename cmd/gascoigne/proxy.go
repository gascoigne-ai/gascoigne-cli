package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/mcpauth"
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

func (r *proxyResult) add(o proxyResult) {
	r.proxied = append(r.proxied, o.proxied...)
	r.direct = append(r.direct, o.direct...)
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
func proxyEntry(exe string, e *object) string {
	typ, _ := e.get("type").(string)
	if e.get("url") != nil && (typ == "http" || typ == "sse" || typ == "") {
		return proxyRemoteEntry(exe, e, typ)
	}
	if typ != "" && typ != "stdio" || e.get("url") != nil || e.get("httpUrl") != nil {
		return "remote: only stdio servers and typed http/sse servers can be proxied"
	}
	cmd, _ := e.get("command").(string)
	if cmd == "" {
		return "no command"
	}
	args, ok := stringsOf(e.get("args"))
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
	e.set("command", newCmd)
	e.set("args", anys(newArgs))
	return ""
}

// proxyRemoteEntry turns a typed http/sse entry into a stdio one running the
// proxy with -url; unproxyServers turns it back.
func proxyRemoteEntry(exe string, e *object, typ string) string {
	url, _ := e.get("url").(string)
	var headers []string
	if h := e.get("headers"); h != nil {
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
	if e.get("oauth") != nil || e.get("headersHelper") != nil {
		return "remote: OAuth and header helpers are not supported"
	}
	if reason := oauthReason(url, headers); reason != "" {
		return reason
	}
	if typ == "" && (e.get("command") != nil || e.get("httpUrl") != nil) {
		return "remote: both url and command or httpUrl"
	}
	for _, k := range []string{"type", "url", "headers"} {
		e.del(k)
	}
	e.set("command", exe)
	e.set("args", anys(remoteProxyArgs(url, typ, headers)))
	return ""
}

// oauthReason is why a remote server stays direct: it asks for an OAuth login
// that neither the entry's headers nor mcp-login provide, so the proxy would
// only break a server that works in the agent today.
func oauthReason(url string, headers []string) string {
	for _, h := range headers {
		if name, _, _ := strings.Cut(h, ":"); strings.EqualFold(strings.TrimSpace(name), "Authorization") {
			return ""
		}
	}
	if _, ok := authStore().Get(url); ok || !mcpauth.NeedsOAuth(context.Background(), url) {
		return ""
	}
	return "OAuth: run gascoigne mcp-login " + mcpauth.Display(url) + ", then install again"
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

func unproxyRemoteEntry(e *object, url, typ string, headers []string) {
	e.del("command")
	e.del("args")
	if typ != "" {
		e.set("type", typ)
	}
	e.set("url", url)
	if len(headers) == 0 {
		return
	}
	h := &object{vals: map[string]any{}}
	for _, kv := range headers {
		k, v, _ := strings.Cut(kv, ":")
		h.set(strings.TrimSpace(k), strings.TrimSpace(v))
	}
	e.set("headers", h)
}

func stringsOf(v any) ([]string, bool) {
	if v == nil {
		return nil, true
	}
	list, ok := v.([]any)
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

func anys(s []string) []any {
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = x
	}
	return out
}

// claudeJSON is Claude Code's state file, home of user and local MCP servers.
func claudeJSON() string {
	home, _ := os.UserHomeDir()
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
			if e, isObj := from.get(name).(*object); isObj && ok(name) && !slices.Contains(disabled, name) {
				if _, seen := servers[name]; !seen {
					order = append(order, name)
				}
				servers[name] = e
			}
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

func obj(v any) *object {
	o, _ := v.(*object)
	return o
}

func stringList(v any) []string {
	s, _ := stringsOf(v)
	return s
}

// toMaps turns parsed JSON (ordered objects) back into maps for encoding.
func toMaps(v any) any {
	switch x := v.(type) {
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
	return v
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

// proxyCodexTOML rewrites a codex config so its stdio servers start through
// the proxy.
func proxyCodexTOML(exe, text string) (string, proxyResult) {
	var r proxyResult
	secs := tomlSections(text)
	for i, sec := range secs {
		name := codexServerName(sec.header)
		if name == "" || name == "gascoigne" {
			continue
		}
		s := codexServerOf(name, sec.body)
		if s.remote || s.cmd == "" {
			r.direct = append(r.direct, name+" (remote: only stdio servers can be proxied)")
			continue
		}
		r.proxied = append(r.proxied, name)
		if _, done := proxiedArgs(s.cmd, s.args); done {
			continue
		}
		cmd, args := proxyCommand(exe, s.cmd, s.args)
		secs[i].body = setTOMLKeys(sec.body, tomlQuote(cmd), tomlArray(args))
	}
	return joinSections(secs), r
}

// unproxyCodexTOML restores servers proxyCodexTOML rewrote.
func unproxyCodexTOML(text string) (string, []string) {
	var names []string
	secs := tomlSections(text)
	for i, sec := range secs {
		name := codexServerName(sec.header)
		if name == "" {
			continue
		}
		s := codexServerOf(name, sec.body)
		orig, ok := proxiedArgs(s.cmd, s.args)
		if !ok {
			continue
		}
		args := ""
		if len(orig) > 1 {
			args = tomlArray(orig[1:])
		}
		secs[i].body = setTOMLKeys(sec.body, tomlQuote(orig[0]), args)
		names = append(names, name)
	}
	return joinSections(secs), names
}

// setTOMLKeys replaces command and args in a table body; empty args removes it.
func setTOMLKeys(body, cmd, args string) string {
	if start, end, _, ok := tomlKey(body, "args"); ok {
		repl := ""
		if args != "" {
			repl = "args = " + args + "\n"
		}
		body = body[:start] + repl + body[end:]
	} else if args != "" {
		body = strings.TrimRight(body, "\n") + "\nargs = " + args + "\n"
	}
	if start, end, _, ok := tomlKey(body, "command"); ok {
		body = body[:start] + "command = " + cmd + "\n" + body[end:]
	}
	return body
}

func joinSections(secs []tomlSection) string {
	var b strings.Builder
	for _, s := range secs {
		b.WriteString(s.header + s.body)
	}
	return b.String()
}

// tomlKey finds "key = value" in a table body: the byte span of the whole
// assignment (through its newline; arrays may span lines) and the value text.
func tomlKey(body, key string) (start, end int, value string, ok bool) {
	pos := 0
	for _, line := range strings.SplitAfter(body, "\n") {
		k, rest, found := strings.Cut(line, "=")
		if found && strings.TrimSpace(k) == key {
			v := strings.TrimLeft(rest, " \t") // a suffix of line
			vstart := pos + len(line) - len(v)
			vend := vstart + valueLen(body[vstart:])
			if i := strings.IndexByte(body[vend:], '\n'); i >= 0 {
				end = vend + i + 1
			} else {
				end = len(body)
			}
			return pos, end, strings.TrimSpace(body[vstart:vend]), true
		}
		pos += len(line)
	}
	return 0, 0, "", false
}

// valueLen is the length of the TOML value at the start of s: a string, an
// array (brackets balanced outside strings) or a bare value up to the line end.
func valueLen(s string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
				if depth == 0 {
					return i + 1
				}
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			depth++
		case c == ']':
			depth--
			if depth == 0 {
				return i + 1
			}
		case c == '#' && depth > 0:
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case (c == '\n' || c == '#') && depth == 0:
			return i
		}
	}
	return len(s)
}

// tomlString decodes a basic ("…") or literal ('…') TOML string.
func tomlString(s string) (string, error) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '\'' && s[len(s)-1] == '\'' {
		return s[1 : len(s)-1], nil
	}
	var out string
	err := json.Unmarshal([]byte(s), &out)
	return out, err
}

// tomlStrings decodes an array of TOML strings.
func tomlStrings(s string) ([]string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "[") || !strings.HasSuffix(s, "]") {
		return nil, false
	}
	var out []string
	rest := strings.TrimSpace(s[1 : len(s)-1])
	for rest != "" {
		if rest[0] == '#' {
			_, rest, _ = strings.Cut(rest, "\n")
			rest = strings.TrimSpace(rest)
			continue
		}
		n := valueLen(rest)
		v, err := tomlString(rest[:n])
		if err != nil {
			return nil, false
		}
		out = append(out, v)
		rest = strings.TrimLeft(strings.TrimSpace(rest[n:]), ",")
		rest = strings.TrimSpace(rest)
	}
	return out, true
}

func tomlQuote(s string) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(s) // strings always encode
	return strings.TrimSpace(b.String())
}

func tomlArray(list []string) string {
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = tomlQuote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}
