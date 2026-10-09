// Package detect classifies text into content types so the right optimizer
// handles it. Detection uses, in order: the command that produced the text,
// a file name or MIME type, then content sniffing.
package detect

import (
	"path"
	"slices"
	"strings"
)

// Content types. Optimizers select blocks by these.
const (
	JSON       = "json"
	NDJSON     = "ndjson"
	HTML       = "html"
	Markdown   = "markdown"
	Diff       = "diff"
	GoTest     = "go-test"
	GoTestJSON = "go-test-json"
	GoBuild    = "go-build"
	GitStatus  = "git-status"
	GitLog     = "git-log"
	GitRemote  = "git-remote" // push, pull, fetch, clone
	JSTest     = "js-test"    // jest, vitest
	TSC        = "tsc"
	NPM        = "npm"
	NX         = "nx"
	Bun        = "bun"
	Grep       = "grep"
	LS         = "ls"
	Code       = "code"
	Log        = "log"
	Text       = "text"
)

type Hints struct {
	Cmd  string // command line that produced the text
	File string // source file path
	MIME string // e.g. from an HTTP Content-Type header
}

// HintsFrom reads hints from payload attrs.
func HintsFrom(attrs map[string]string) Hints {
	return Hints{Cmd: attrs["cmd"], File: attrs["file"], MIME: attrs["mime"]}
}

func Detect(text string, h Hints) string {
	if t := FromCmd(h.Cmd); t != "" {
		return t
	}
	if t := FromFile(h.File); t != "" {
		return t
	}
	if t := FromMIME(h.MIME); t != "" {
		return t
	}
	return Sniff(text)
}

// wrappers are skipped to find the real command.
var wrappers = map[string]bool{"sudo": true, "time": true, "env": true, "command": true, "exec": true, "nice": true}

// Argv splits cmd and drops env assignments and wrapper commands.
func Argv(cmd string) []string {
	args := strings.Fields(cmd)
	for len(args) > 0 && (strings.Contains(args[0], "=") || wrappers[path.Base(args[0])]) {
		args = args[1:]
	}
	if len(args) > 0 {
		args[0] = path.Base(args[0])
	}
	return args
}

// FromCmd maps a command line to a content type; "" when unknown.
func FromCmd(cmd string) string {
	args := Argv(cmd)
	if len(args) == 0 {
		return ""
	}
	sub := subcommand(args)
	switch args[0] {
	case "go":
		switch sub {
		case "test":
			if hasFlag(args, "-json") {
				return GoTestJSON
			}
			return GoTest
		case "build", "vet", "install":
			return GoBuild
		}
	case "git":
		switch sub {
		case "status":
			return GitStatus
		case "log":
			return GitLog
		case "diff", "show":
			return Diff
		case "push", "pull", "fetch", "clone":
			return GitRemote
		case "grep":
			return Grep
		}
	case "npm", "pnpm", "yarn":
		return NPM
	case "bun", "bunx":
		return Bun
	case "nx":
		return NX
	case "npx":
		if len(args) > 1 {
			return FromCmd(strings.Join(args[1:], " "))
		}
	case "tsc":
		return TSC
	case "jest", "vitest":
		return JSTest
	case "grep", "egrep", "rg", "ag", "ack":
		return Grep
	case "ls", "eza", "exa":
		return LS
	case "cat", "head", "tail", "bat", "less":
		if f := lastArg(args); f != "" {
			return FromFile(f)
		}
	}
	return ""
}

// valueFlags take a separate value argument before the subcommand (git -C dir status).
var valueFlags = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--prefix": true}

func subcommand(args []string) string {
	for i := 1; i < len(args); i++ {
		switch a := args[i]; {
		case valueFlags[a]:
			i++
		case !strings.HasPrefix(a, "-"):
			return a
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

func lastArg(args []string) string {
	for _, a := range slices.Backward(args[1:]) {
		if !strings.HasPrefix(a, "-") {
			return a
		}
	}
	return ""
}

var extTypes = map[string]string{
	".json": JSON, ".jsonl": NDJSON, ".ndjson": NDJSON,
	".html": HTML, ".htm": HTML,
	".md": Markdown, ".log": Log,
	".diff": Diff, ".patch": Diff,
}

// FromFile maps a file name to a content type; "" when unknown.
func FromFile(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if t, ok := extTypes[ext]; ok {
		return t
	}
	if _, ok := langByExt[ext]; ok {
		return Code
	}
	return ""
}

// FromMIME maps a MIME type to a content type; "" when unknown.
func FromMIME(mime string) string {
	mime, _, _ = strings.Cut(strings.ToLower(mime), ";")
	switch strings.TrimSpace(mime) {
	case "application/json", "application/problem+json":
		return JSON
	case "application/x-ndjson", "application/jsonl":
		return NDJSON
	case "text/html", "application/xhtml+xml":
		return HTML
	case "text/markdown":
		return Markdown
	}
	return ""
}
