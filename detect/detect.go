// Package detect classifies text into content types so the right optimizer
// handles it. A type is "family/dialect": the family names what the output is
// for (test, build, pkg, table, k8s, cloud…) and picks the engine; the dialect
// names the tool or format and tunes it. Detection uses, in order: the command
// that produced the text, a file name or MIME type, then content sniffing.
// Dialects are declared as specs in this package, one file per family.
package detect

import (
	"path"
	"slices"
	"strings"
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
	for _, t := range []string{FromCmd(h.Cmd), FromFile(h.File), FromMIME(h.MIME)} {
		if t != "" {
			return refine(t, text)
		}
	}
	t := Sniff(text)
	if t == HTML && h.Cmd != "" && !fetchesPage(h.Cmd) {
		return Code // markup printed by a file read or a script is source, not a page
	}
	return t
}

// fetchers are the programs whose output is a page when it looks like one.
var fetchers = map[string]bool{"curl": true, "wget": true, "http": true, "https": true, "xh": true, "lynx": true, "w3m": true, "links": true}

// fetchesPage reports whether cmd fetches from the web: only then is HTML
// output a page to convert to markdown.
func fetchesPage(cmd string) bool {
	args := Argv(firstStage(lastCommand(cmd)))
	return len(args) > 0 && fetchers[args[0]]
}

// refine swaps a generic type for a spec refining it whose sniffer matches.
func refine(t, text string) string {
	for _, s := range specs {
		if slices.Contains(s.Refines, t) && s.Sniff(text, sample(text)) {
			return s.Type
		}
	}
	return t
}

// Family is the family part of a type: "test" for "test/jest".
func Family(typ string) string {
	family, _, _ := strings.Cut(typ, "/")
	return family
}

// Dialect is the dialect part of a type: "jest" for "test/jest".
func Dialect(typ string) string {
	_, dialect, _ := strings.Cut(typ, "/")
	return dialect
}

var (
	// wrappers are skipped to find the real command.
	wrappers = map[string]bool{"sudo": true, "time": true, "env": true, "command": true, "exec": true, "nice": true, "nohup": true}
	// runners execute the package named next: "npx nx run-many" is nx.
	runners = map[string]bool{"npx": true, "bunx": true, "pnpx": true, "uvx": true, "pipx": true}
)

// Argv splits cmd, drops env assignments and wrappers, unwraps package
// runners and reduces the program to its base name.
func Argv(cmd string) []string {
	args := strings.Fields(cmd)
	for len(args) > 0 && (strings.Contains(args[0], "=") || wrappers[path.Base(args[0])] || runners[path.Base(args[0])] && len(args) > 1) {
		args = args[1:]
		for len(args) > 1 && strings.HasPrefix(args[0], "-") { // npx --yes nx …
			args = args[1:]
		}
	}
	if len(args) > 0 {
		args[0] = path.Base(args[0])
	}
	// "uv run pytest", "bundle exec rspec", "npm exec vitest", "python -m pytest".
	if len(args) > 2 && (execs[args[0]+" "+args[1]] || strings.HasPrefix(args[0], "python") && args[1] == "-m") {
		return Argv(strings.Join(args[2:], " "))
	}
	return args
}

// execs run the command that follows them.
var execs = map[string]bool{
	"uv run": true, "poetry run": true, "pipenv run": true, "pdm run": true, "hatch run": true,
	"bundle exec": true, "npm exec": true, "pnpm exec": true, "pnpm dlx": true, "yarn exec": true, "yarn dlx": true,
}

// valueFlags take a separate value before positional arguments
// (git -C dir status, kubectl -n ns get pods, aws --profile p s3 ls).
var valueFlags = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--prefix": true,
	"-n": true, "--namespace": true, "--context": true, "--kubeconfig": true,
	"--profile": true, "--region": true, "--project": true, "--subscription": true,
	"-o": true, "--output": true, "--format": true,
}

// positional returns arguments after the program that are not flags or flag values.
func positional(args []string) []string {
	var out []string
	for i := 1; i < len(args); i++ {
		switch a := args[i]; {
		case valueFlags[a]:
			i++
		case !strings.HasPrefix(a, "-"):
			out = append(out, a)
		}
	}
	return out
}

func hasFlag(args []string, flag string) bool {
	return slices.ContainsFunc(args, func(a string) bool { return a == flag || strings.HasPrefix(a, flag+"=") })
}

// flagValue returns the value of -o yaml, -o=yaml, -oyaml style flags.
func flagValue(args []string, names ...string) string {
	for i, a := range args {
		for _, n := range names {
			switch {
			case a == n && i+1 < len(args):
				return args[i+1]
			case strings.HasPrefix(a, n+"="):
				return a[len(n)+1:]
			case len(n) == 2 && strings.HasPrefix(a, n) && len(a) > 2 && a[2] != '-':
				return a[2:]
			}
		}
	}
	return ""
}

// tools whose subcommands are worth reporting ("go test", "git log", "kubectl get").
var subcommandTools = map[string]int{
	"go": 1, "git": 1, "npm": 1, "pnpm": 1, "yarn": 1, "bun": 1, "nx": 1, "turbo": 1,
	"cargo": 1, "docker": 1, "podman": 1, "kubectl": 1, "helm": 1, "gh": 2, "make": 1, "pip": 1,
	"uv": 1, "poetry": 1, "terraform": 1, "tofu": 1, "pulumi": 1, "dotnet": 1, "deno": 1,
	"aws": 2, "gcloud": 3, "az": 2, "systemctl": 1, "journalctl": 0, "mvn": 1, "gradle": 1,
}

// Command names a command line for reports: the program plus its subcommands
// for multi-command tools ("go test", "aws s3 ls", "npx nx run-many" → "nx run-many").
func Command(cmd string) string {
	args := Argv(lastCommand(cmd))
	if len(args) == 0 {
		return ""
	}
	pos := positional(args)
	n := min(subcommandTools[args[0]], len(pos))
	return strings.Join(append([]string{args[0]}, pos[:n]...), " ")
}

// lastArg is the last positional argument (the file of cat, head, tail).
func lastArg(args []string) string {
	pos := positional(args)
	if len(pos) == 0 {
		return ""
	}
	return pos[len(pos)-1]
}

// lastCommand is the command of a line that does the work: "cd api && go
// test ./..." names go test, not cd. Leading cd/pushd/export steps of an
// && or ; list are skipped.
func lastCommand(line string) string {
	for {
		i := strings.IndexAny(line, "&;")
		if i < 0 {
			return line
		}
		first := strings.Fields(line[:i])
		if len(first) == 0 || !setup[first[0]] {
			return line
		}
		line = strings.TrimLeft(line[i:], "&; ")
	}
}

// firstStage is the command a line's output starts with: cut at the first
// pipe, ; or & outside quotes. Later stages of a pipe only filter that output,
// so they must not decide what it is (the last word of "sed -n 1,9p a.html |
// cut -c1-80" is not a file).
func firstStage(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
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
		case c == '|' || c == ';' || c == '&' || c == '\n':
			return line[:i]
		}
	}
	return line
}

var setup = map[string]bool{"cd": true, "pushd": true, "export": true, "source": true, ".": true, "set": true}
