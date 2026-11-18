package main

import (
	"encoding/json"
	"strings"
)

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
func valueLen(text string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			end := quoteEnd(text, i, quote)
			if end < 0 {
				return len(text)
			}
			i, quote = end, 0
			if depth == 0 {
				return i + 1
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
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case (c == '\n' || c == '#') && depth == 0:
			return i
		}
	}
	return len(text)
}

// quoteEnd is the index of the quote that closes the string being read at
// text[from] (backslash escapes only in basic strings), -1 without one.
func quoteEnd(text string, from int, quote byte) int {
	for i := from; i < len(text); i++ {
		switch {
		case text[i] == '\\' && quote == '"':
			i++
		case text[i] == quote:
			return i
		}
	}
	return -1
}

// tomlString decodes a basic ("…") or literal ('…') TOML string.
func tomlString(text string) (string, error) {
	text = strings.TrimSpace(text)
	if len(text) >= 2 && text[0] == '\'' && text[len(text)-1] == '\'' {
		return text[1 : len(text)-1], nil
	}
	var out string
	err := json.Unmarshal([]byte(text), &out)
	return out, err
}

// tomlStrings decodes an array of TOML strings.
func tomlStrings(text string) ([]string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "[") || !strings.HasSuffix(text, "]") {
		return nil, false
	}
	var out []string
	rest := strings.TrimSpace(text[1 : len(text)-1])
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

func tomlQuote(text string) string {
	encoded, _ := encodeJSON(text) // strings always encode
	return strings.TrimSpace(string(encoded))
}

func tomlArray(list []string) string {
	q := make([]string, len(list))
	for i, s := range list {
		q[i] = tomlQuote(s)
	}
	return "[" + strings.Join(q, ", ") + "]"
}
