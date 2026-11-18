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
// prints its hint first). Commands may be joined by && or ;, and only
// head/tail may follow a pipe there; any other pipeline ending the line gets
// gascoigne run appended (wrapPipeline), so a filter never sees optimized
// text. Redirects, subshells, other lists, loops and
// conditionals are left alone.
func wrapCommand(line, prefix string) (string, bool) {
	line = strings.TrimSpace(line)
	if ownCommand(line) || strings.Contains(line, "\n") {
		return line, false
	}
	parts, seps := segments(line)
	var out strings.Builder
	wrapped := false
	for i, part := range parts {
		if i > 0 {
			out.WriteString(map[string]string{";": "; ", "&&": " && "}[seps[i-1]])
		}
		body, pipe := splitPipe(part)                    // this command's own trailing | head, | tail
		body, merged := strings.CutSuffix(body, " 2>&1") // exec merges stderr anyway
		if hasShellOperator(body) {
			if piped, ok := wrapPipeline(part, prefix); ok && i == len(parts)-1 && !strings.Contains(line, "pipefail") {
				out.WriteString(piped)
				return out.String(), true
			}
			return line, false
		}
		if f := strings.Fields(body); len(f) > 0 && shellKeywords[f[0]] {
			return line, false // loops, conditionals, groups: wrapping a piece could change what runs
		}
		env, cmd := detect.SplitEnv(body)
		if passthrough(cmd) {
			out.WriteString(part)
			continue
		}
		wrapped = true
		run := prefix + " -- "
		if watched(cmd) {
			run = prefix + " -watch -- "
		}
		if slices.Contains(strings.Fields(pipe), "head") {
			run = prefix + " -attr " + stdoutexporter.AttrHint + "=" + stdoutexporter.HintTop + " -- " // head would cut a hint at the end
		}
		out.WriteString(env + run + cmd)
		if merged {
			out.WriteString(" 2>&1")
		}
		out.WriteString(pipe)
	}
	if !wrapped {
		return line, false
	}
	return out.String(), true
}

// wrapPipeline appends gascoigne run to a pipeline of plain commands, so each
// stage sees raw text and only the final output is optimized. The line's
// status stays the last original stage's (bash PIPESTATUS, zsh pipestatus),
// not that of gascoigne run. Streaming stages and anything beyond
// plain stages (redirects, substitutions, loops) are left alone.
func wrapPipeline(part, prefix string) (string, bool) {
	stages := splitStages(part)
	if len(stages) < 2 {
		return "", false
	}
	var last string
	for _, s := range stages {
		s, _ = strings.CutSuffix(strings.TrimSpace(s), " 2>&1")
		f := strings.Fields(s)
		_, last = detect.SplitEnv(s)
		if len(f) == 0 || hasShellOperator(s) || shellKeywords[f[0]] || passthrough(last) {
			return "", false
		}
	}
	run := strings.Replace(prefix, " exec", " run", 1)
	n := len(stages) - 1
	return fmt.Sprintf("%s | %s -attr cmd=%s; (exit ${PIPESTATUS[%d]:-${pipestatus[%d]}})",
		part, run, shellQuote(last), n, n+1), true
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

// segments splits a command list at && and ; outside quotes: the trimmed
// commands, and the separator after each but the last.
func segments(command string) (parts, seps []string) {
	var quote byte
	start := 0
	cut := func(end int, sep string) {
		parts = append(parts, strings.TrimSpace(command[start:end]))
		seps = append(seps, sep)
	}
	for i := 0; i < len(command); i++ {
		switch c := command[i]; {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == ';':
			cut(i, ";")
			start = i + 1
		case c == '&' && i+1 < len(command) && command[i+1] == '&':
			cut(i, "&&")
			i++
			start = i + 1
		}
	}
	parts = append(parts, strings.TrimSpace(command[start:]))
	return parts, seps
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
	for s := range strings.FieldsFuncSeq(line, func(char rune) bool { return strings.ContainsRune("|;&", char) }) {
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
