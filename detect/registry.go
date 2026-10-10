package detect

import (
	"cmp"
	"path"
	"slices"
	"strings"
)

// Spec declares a dialect: how to recognize it from a command, a file name,
// a MIME type or its content.
type Spec struct {
	Type string // family/dialect
	// Cmds are patterns "program sub sub -flag": the program, positional
	// subcommands in order ("*" matches any) and flags that must be present.
	// The most specific matching pattern across all specs wins.
	Cmds []string
	// CmdFunc matches what patterns can't (flag values like -o yaml); a match
	// counts as more specific than any pattern of the same program.
	CmdFunc func(args []string) bool
	Files   []string // base name globs: "*.tf", "Dockerfile"
	MIME    []string
	Sniff   Sniffer
	// Priority orders sniffing, lower first: specific formats before generic
	// ones (logs, tables, text).
	Priority int
	// Refines lists generic types (from a file name, MIME type or command)
	// this spec replaces when its sniffer matches: a .yaml file holding
	// Kubernetes manifests is k8s/manifest, not data/yaml.
	Refines []string
}

// Sniffer recognizes content from the full text and sampled, ANSI-stripped lines.
type Sniffer func(text string, lines []string) bool

var specs []Spec

// register adds dialect specs; family files call it from init.
func register(s ...Spec) {
	specs = append(specs, s...)
	slices.SortStableFunc(specs, func(a, b Spec) int { return cmp.Compare(a.Priority, b.Priority) })
}

// Types lists every registered dialect type.
func Types() []string {
	var out []string
	for _, s := range specs {
		out = append(out, s.Type)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// fileReaders print a file: their type comes from the file name.
var fileReaders = map[string]bool{"cat": true, "head": true, "tail": true, "bat": true, "less": true, "more": true, "sed": true}

// sourceMarkup are file types that are pages when fetched but source when
// read from disk: an agent reading index.html means to edit it.
var sourceMarkup = map[string]bool{HTML: true}

// FromCmd maps a command line to a type; "" when unknown.
func FromCmd(cmd string) string {
	args := Argv(cmd)
	if len(args) == 0 {
		return ""
	}
	if fileReaders[args[0]] {
		if t := FromFile(lastArg(args)); !sourceMarkup[t] {
			return t
		}
		return Code
	}
	best, score := "", 0
	for _, s := range specs {
		if n := s.matchCmd(args); n > score {
			best, score = s.Type, n
		}
	}
	return best
}

func (s Spec) matchCmd(args []string) int {
	best := 0
	for _, p := range s.Cmds {
		best = max(best, matchPattern(p, args))
	}
	if s.CmdFunc != nil && s.CmdFunc(args) {
		best = max(best, 100)
	}
	return best
}

// matchPattern scores how specifically pattern matches args; 0 = no match.
func matchPattern(pattern string, args []string) int {
	toks := strings.Fields(pattern)
	if ok, _ := path.Match(toks[0], args[0]); !ok {
		return 0
	}
	pos := positional(args)
	sub := 0
	for _, t := range toks[1:] {
		if strings.HasPrefix(t, "-") {
			if !hasFlag(args, t) {
				return 0
			}
			continue
		}
		if sub >= len(pos) {
			return 0
		}
		if ok, _ := path.Match(t, pos[sub]); !ok {
			return 0
		}
		sub++
	}
	return len(toks)
}

// FromFile maps a file name to a type; "" when unknown.
func FromFile(name string) string {
	if name == "" {
		return ""
	}
	base := strings.ToLower(path.Base(name))
	for _, s := range specs {
		for _, g := range s.Files {
			if ok, _ := path.Match(g, base); ok {
				return s.Type
			}
		}
	}
	if Lang("", Hints{File: name}) != "" {
		return Code
	}
	return ""
}

// FromMIME maps a MIME type to a type; "" when unknown.
func FromMIME(mime string) string {
	mime, _, _ = strings.Cut(strings.ToLower(mime), ";")
	mime = strings.TrimSpace(mime)
	for _, s := range specs {
		if slices.Contains(s.MIME, mime) {
			return s.Type
		}
	}
	return ""
}
