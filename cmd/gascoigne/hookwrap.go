package main

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/exporter/stdoutexporter"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// wrapCommand routes each command of a shell line through gascoigne exec
// (except builtins, quiet and interactive commands, see passthrough): "cd api && go test ./..." becomes
// "cd api && gascoigne exec -- go test ./...", and "go test ./... | tail -5"
// becomes "gascoigne exec -- go test ./... | tail -5" (before head, exec
// prints its hint first). The line is rewritten segment by segment, a segment
// being a command between ;, && and newlines. Only head/tail may follow a pipe
// in a wrapped command; any other plain pipeline gets gascoigne run appended
// (wrapPipeline), so a filter never sees optimized text. A segment that cannot
// be wrapped (redirect, substitution, pipeline of builtins) stays verbatim and
// the others are wrapped. Lines with ||, background runs, heredocs, pipefail,
// subshells, loops, conditionals or groups are left alone as a whole.
func wrapCommand(line, prefix string) (string, bool) {
	line = strings.TrimSpace(line)
	parts, seps, ok := segments(line)
	if !ok || ownCommand(line) || strings.Contains(line, "pipefail") || slices.ContainsFunc(parts, startsCompound) {
		return line, false
	}
	commands := 0
	for _, part := range parts {
		if part != "" {
			commands++
		}
	}
	multi := commands > 1
	var out strings.Builder
	wrapped := false
	for i, part := range parts {
		if i > 0 {
			out.WriteString(map[string]string{";": "; ", "&&": " && ", "\n": "\n"}[seps[i-1]])
		}
		text, done := wrapSegment(part, prefix, multi, segmentPos{last: i == len(parts)-1, afterAnd: i > 0 && seps[i-1] == "&&"})
		out.WriteString(text)
		wrapped = wrapped || done
	}
	if !wrapped {
		return line, false
	}
	return out.String(), true
}

// segmentPos says where a segment stands in its line: a pipeline is only safe
// without braces at the end of the line after a ; or newline (or alone).
type segmentPos struct{ last, afterAnd bool }

func (pos segmentPos) bare() bool { return pos.last && !pos.afterAnd }

// startsCompound reports segments that open or close a loop, conditional or group.
func startsCompound(part string) bool {
	f := strings.Fields(part)
	return len(f) > 0 && shellKeywords[f[0]] // wrapping a piece could change what runs
}

// wrapSegment is one command of a list as it should run, and whether it
// changed: wrapped, or verbatim when gascoigne cannot help (or would not pay).
func wrapSegment(part, prefix string, multi bool, pos segmentPos) (string, bool) {
	body, pipe := splitPipe(part)                    // this command's own trailing | head, | tail
	body, merged := strings.CutSuffix(body, " 2>&1") // exec merges stderr anyway
	if hasShellOperator(body) {
		return wrapPipeline(part, prefix, multi, pos)
	}
	env, cmd := detect.SplitEnv(body)
	if passthrough(cmd) || !worthWrapping(cmd, multi) {
		return part, false
	}
	run := prefix + " -- "
	if watched(cmd) {
		run = prefix + " -watch -- "
	}
	if slices.Contains(strings.Fields(pipe), "head") {
		run = prefix + " -attr " + stdoutexporter.AttrHint + "=" + stdoutexporter.HintTop + " -- " // head would cut a hint at the end
	}
	if merged {
		cmd += " 2>&1"
	}
	return env + run + cmd + pipe, true
}

// worthWrapping is the payoff gate: a hint costs tokens in every segment, so
// in a list only commands of a known dialect are wrapped. A single command
// is wrapped whatever it is, the output may still be recognized by content.
func worthWrapping(cmd string, multi bool) bool {
	return !multi || detect.FromCmd(cmd) != ""
}

// wrapPipeline appends gascoigne run to a pipeline of plain commands, so each
// stage sees raw text and only the final output is optimized. The line's
// status stays the last original stage's (bash PIPESTATUS, zsh pipestatus),
// not that of gascoigne run. A pipeline that is not at the end of the line, or
// follows &&, goes in { ...; } so the status line reads right after it and the
// list (&&, ;, set -e) sees the original status. Streaming stages and anything
// beyond plain stages (redirects, substitutions, loops) are left alone, as is
// a pipeline whose last stage is unknown in a list (payoff gate).
func wrapPipeline(part, prefix string, multi bool, pos segmentPos) (string, bool) {
	stages := splitStages(part)
	if len(stages) < 2 {
		return part, false
	}
	var last string
	for _, s := range stages {
		s, _ = strings.CutSuffix(strings.TrimSpace(s), " 2>&1")
		f := strings.Fields(s)
		_, last = detect.SplitEnv(s)
		if len(f) == 0 || hasShellOperator(s) || shellKeywords[f[0]] || passthrough(last) {
			return part, false
		}
	}
	if !worthWrapping(last, multi) {
		return part, false
	}
	run := strings.Replace(prefix, " exec", " run", 1)
	n := len(stages) - 1
	text := fmt.Sprintf("%s | %s -attr cmd=%s; (exit ${PIPESTATUS[%d]:-${pipestatus[%d]}})",
		part, run, shellQuote(last), n, n+1)
	if !pos.bare() {
		text = "{ " + text + "; }"
	}
	return text, true
}

// hasShellOperator reports whether s holds something that makes a piece of a
// line more than one plain command: a pipe, a redirect, a background &, a
// backtick or $( substitution, or a quote that never closes. Characters inside
// quotes are literal (grep 'a\|b'), except a substitution inside double quotes,
// which still runs; a backslash escapes the next character.
func hasShellOperator(command string) bool {
	var quote byte
	for i := 0; i < len(command); i++ {
		c := command[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\':
				i++
			case c == '`' || c == '$' && i+1 < len(command) && command[i+1] == '(':
				return true
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\':
			i++
		case strings.IndexByte("|&<>`\n", c) >= 0 || c == '$' && i+1 < len(command) && command[i+1] == '(':
			return true
		}
	}
	return quote != 0
}

// shellKeywords start compound commands, whose parts are not separate commands.
var shellKeywords = map[string]bool{
	"if": true, "then": true, "else": true, "elif": true, "fi": true, "for": true, "while": true, "until": true,
	"do": true, "done": true, "case": true, "esac": true, "select": true, "function": true, "{": true, "}": true,
}

// segments splits a command list at ;, && and newlines outside quotes,
// substitutions ($( ), backticks, ${ }) and escapes: the trimmed commands, and
// the separator (";", "&&" or "\n") after each but the last. ok is false when
// the line holds something that makes a cut unsafe: ||, a background &, a
// heredoc, a bare ( or ), or a quote or substitution that never closes.
func segments(command string) (parts, seps []string, ok bool) {
	start := 0
	cut := func(end int, sep string) {
		parts = append(parts, strings.TrimSpace(command[start:end]))
		seps = append(seps, sep)
	}
	for i := 0; i < len(command); i++ {
		c := command[i]
		var after byte // the next character
		if i+1 < len(command) {
			after = command[i+1]
		}
		switch {
		case c == '\\':
			i++ // also a backslash-newline continuation
		case c == '\'', c == '"', c == '`', c == '$' && (after == '(' || after == '{'):
			if i = skipQuoted(command, i); i < 0 {
				return nil, nil, false
			}
		case c == '(' || c == ')', c == '<' && after == '<', c == '|' && after == '|':
			return nil, nil, false
		case c == '&' && after == '&':
			cut(i, "&&")
			i++
			start = i + 1
		case c == '&':
			if after != '>' && (i == 0 || command[i-1] != '>' && command[i-1] != '<') {
				return nil, nil, false // background, or the end of |&
			}
		case c == ';':
			cut(i, ";")
			start = i + 1
		case c == '\n':
			cut(i, "\n")
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(command[start:]))
	return parts, seps, true
}

// skipQuoted returns the index of the last character of the quote or
// substitution that opens at command[open], or -1 when it never closes.
func skipQuoted(command string, open int) int {
	switch command[open] {
	case '\'':
		end := strings.IndexByte(command[open+1:], '\'')
		if end < 0 {
			return -1
		}
		return open + 1 + end
	case '"':
		return skipUntil(command, open+1, '"', true)
	case '`':
		return skipUntil(command, open+1, '`', false)
	case '$':
		if command[open+1] == '{' {
			return skipUntil(command, open+2, '}', false)
		}
		return skipUntil(command, open+2, ')', false)
	}
	return -1
}

// skipUntil scans from index from for closer, stepping over what nests: escapes, quotes,
// substitutions and parentheses. Inside double quotes (literal) only
// substitutions and escapes nest. It returns the index of closer, or -1.
func skipUntil(command string, from int, closer byte, literal bool) int {
	for i := from; i < len(command); i++ {
		c := command[i]
		var after byte
		if i+1 < len(command) {
			after = command[i+1]
		}
		switch {
		case c == closer:
			return i
		case c == '\\':
			i++
		case c == '(' && !literal:
			if i = skipUntil(command, i+1, ')', false); i < 0 {
				return -1
			}
		case c == '`' || c == '$' && (after == '(' || after == '{') || !literal && (c == '\'' || c == '"'):
			if i = skipQuoted(command, i); i < 0 {
				return -1
			}
		}
	}
	return -1
}

// splitPipe cuts trailing "| head …" and "| tail …" stages off a command; pipe is
// empty when there are none or another command follows a pipe. Pipes inside
// quotes are not stage separators.
func splitPipe(line string) (body, pipe string) {
	stages := splitStages(line)
	if len(stages) == 1 {
		return line, ""
	}
	for _, s := range stages[1:] {
		args := strings.Fields(s)
		if len(args) == 0 || args[0] != "head" && args[0] != "tail" || streams(s) {
			return line, ""
		}
		for _, a := range args[1:] {
			if strings.Trim(a, "-+ncq0123456789") != "" {
				return line, ""
			}
		}
	}
	body = strings.TrimSpace(stages[0])
	return body, strings.TrimPrefix(line, body)
}

// splitStages splits s at pipes outside quotes (and not escaped).
func splitStages(command string) []string {
	var stages []string
	var quote byte
	start := 0
	for i := 0; i < len(command); i++ {
		switch c := command[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\\' && quote == '"' {
				i++
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '\\':
			i++
		case c == '|':
			stages = append(stages, command[start:i])
			start = i + 1
		}
	}
	return append(stages, command[start:])
}

// ownCommand reports lines that run gascoigne itself or ask for raw output.
func ownCommand(line string) bool {
	if strings.Contains(line, "GASCOIGNE_RAW") {
		return true
	}
	for s := range strings.FieldsFuncSeq(line, func(char rune) bool { return strings.ContainsRune("|;&\n", char) }) {
		if args := detect.Argv(s); len(args) > 0 && args[0] == "gascoigne" {
			return true
		}
	}
	return false
}

// passthrough reports commands exec should not run: shell builtins (cd,
// export: they must change this shell), commands that print little or
// nothing (each would only add a bypassed run to the stats), interactive
// ones and those that run until stopped. Unknown programs are wrapped: their
// output is detected by content (JSON, logs, tables).
func passthrough(cmd string) bool {
	args := detect.Argv(cmd)
	if len(args) == 0 {
		return true
	}
	if f := strings.Fields(cmd); quiet[f[0]] { // "command ls": a builtin, no program exec could run
		return true
	}
	name := filepath.Base(args[0])
	if quiet[name] || len(args) == 1 && repl[name] {
		return true
	}
	return streams(cmd) && !watched(cmd)
}

// watched reports build tools in watch mode, whose output exec -watch
// prints a rebuild at a time.
func watched(cmd string) bool {
	args := detect.Argv(cmd)
	if len(args) < 2 || detect.Family(detect.FromCmd(cmd)) != detect.FamilyBuild {
		return false
	}
	return slices.ContainsFunc(args[1:], func(arg string) bool {
		return arg == "-w" || arg == "--watch" || arg == "watch" || arg == "serve"
	})
}

var quiet = map[string]bool{
	// builtins
	"cd": true, "pushd": true, "popd": true, "export": true, "unset": true, "set": true, "source": true, ".": true,
	"alias": true, "unalias": true, "exit": true, "return": true, "eval": true, "exec": true, "read": true,
	"wait": true, "trap": true, "ulimit": true, "umask": true, "shift": true, "local": true, "declare": true,
	"typeset": true, "readonly": true, "shopt": true, "hash": true, "type": true, "command": true, "builtin": true,
	"time": true, "jobs": true, "fg": true, "bg": true, "disown": true, ":": true, "true": true, "false": true, "test": true, "[": true, "[[": true,
	// little output
	"echo": true, "printf": true, "sleep": true, "mkdir": true, "rmdir": true, "rm": true, "mv": true, "cp": true,
	"touch": true, "chmod": true, "chown": true, "ln": true, "which": true, "pwd": true, "clear": true, "kill": true,
	// interactive
	"vi": true, "vim": true, "nvim": true, "nano": true, "emacs": true, "less": true, "more": true, "watch": true,
}

// repl: programs that read commands interactively when run without arguments.
var repl = map[string]bool{"python": true, "python3": true, "node": true, "irb": true, "ghci": true, "psql": true, "mysql": true, "sqlite3": true, "bash": true, "sh": true, "zsh": true}

// streams reports commands that run until stopped (followed logs, watchers,
// dev servers): exec shows output only when the command exits.
func streams(cmd string) bool {
	args := detect.Argv(cmd)
	if len(args) == 0 {
		return false
	}
	if args[0] == "vite" && (len(args) == 1 || strings.HasPrefix(args[1], "-")) {
		return true // the dev server
	}
	follow := args[0] == "tail" || args[0] == "journalctl" || strings.Contains(cmd, "logs")
	for _, a := range args[1:] {
		switch {
		case a == "--follow" || a == "--watch" || a == "watch" || a == "dev" || a == "serve" || a == "start" || a == "up":
			return true
		case a == "-f" && follow:
			return true
		}
	}
	return false
}
