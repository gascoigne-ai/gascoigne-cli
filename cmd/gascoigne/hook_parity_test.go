//go:build unix

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// execStub stands in for `gascoigne exec`: it drops the flags up to "--" and
// runs what follows, like exec does without optimizing. exec starts a
// program, so a builtin or keyword there fails as it would for real.
const execStub = `#!/bin/sh
[ "$1" = exec ] && shift
while [ $# -gt 0 ]; do
	case "$1" in
	--) shift; break ;;
	-attr|-agent|-session|-model) shift 2 ;;
	*) shift ;;
	esac
done
case "$(command -v "$1")" in
/*) exec "$@" ;;
esac
echo "gascoigne: $1: executable file not found" >&2
exit 127
`

// run is the combined output and exit code of line under sh in dir.
func runShell(t *testing.T, dir, bin, line string) (string, int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", line)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "LC_ALL=C", "HOME=/home/parity", "REPLY=from-env")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		return string(out), exitErr.ExitCode()
	case err != nil:
		t.Fatalf("%q: %v", line, err)
	}
	return string(out), 0
}

// The rewritten line must behave like the original: same output, same exit
// status, whatever quoting, env assignments or list operators it holds.
func TestWrapCommandShellParity(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "gascoigne"), []byte(execStub), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{"a.txt": "one\ntwo\nthree\n", "b c.txt": "x y\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, line string
		wrapped    bool // the hook should rewrite the line
	}{
		// env assignments
		{"env unquoted", `PG=aaaa printenv PG`, true},
		{"env double quoted", `PG="aaaa" printenv PG`, true},
		{"env single quoted", `PG='aaaa' printenv PG`, true},
		{"env quoted args after", `PG="aaaa" sh -c 'echo "$PG" "$1"' sh "hjj"`, true},
		{"env value with space", `PG="a b" printenv PG`, true},
		{"env single quoted space", `PG='a  b' printenv PG`, true},
		{"env backslash space", `PG=a\ b printenv PG`, true},
		{"env escaped quote", `PG="a \"q\" b" printenv PG`, true},
		{"env apostrophe in double quotes", `PG="it's" printenv PG`, true},
		{"env double quote in single quotes", `PG='say "hi"' printenv PG`, true},
		{"env empty", `PG="" sh -c 'echo "[$PG]"'`, true},
		{"env empty unquoted", `PG= sh -c 'echo "[$PG]"'`, true},
		{"env expands var", `PG="$HOME/x" printenv PG`, true},
		{"env expands var unquoted", `PG=$HOME printenv PG`, true},
		{"env single quoted no expand", `PG='$HOME' printenv PG`, true},
		{"env separators in quotes", `PG="a;b && c | d" printenv PG`, true},
		{"env glob in quotes", `PG="*.txt" printenv PG`, true},
		{"env equals in value", `PG="k=v" printenv PG`, true},
		{"env two", `A="x y" B=2 sh -c 'echo "$A|$B"'`, true},
		{"env three mixed quoting", `A=1 B='2 2' C="3 3" sh -c 'echo "$A|$B|$C"'`, true},
		{"env underscore and digits", `_PG_1="v" printenv _PG_1`, true},
		{"env then unknown program is not hidden", `PG="x" mytool-xyz 2>/dev/null; echo $?`, false},
		{"env visible only to command", `PG="v" sh -c 'echo "$PG"'; sh -c 'echo "[$PG]"'`, false}, // unknown programs in a list stay as they are (payoff gate)
		{"env overrides inherited", `REPLY="mine" printenv REPLY`, true},
		{"env tab-free value with hash", `PG="a#b" printenv PG`, true},
		{"env dollar paren in quotes literal", `PG='$(echo x)' printenv PG`, true},
		{"env then flag with equals", `PG="v" sh -c 'echo "$PG $0"' --x=y`, true},

		// quoting without env
		{"double quoted arg", `sh -c 'echo "$0"' "a b"`, true},
		{"single quoted arg", `sh -c 'echo "$0"' 'a  b'`, true},
		{"glob in quotes stays literal", `sh -c 'echo "$0"' "*.txt"`, true},
		{"glob unquoted expands", `ls *.txt`, true},
		{"quoted file with space", `cat "b c.txt"`, true},
		{"escaped space file", `cat b\ c.txt`, true},
		{"flag with equals", `sort --key=1 a.txt`, true},
		{"assignment-looking arg", `sh -c 'echo "$0"' x=y`, true},
		{"assignment-looking arg after env", `A=1 sh -c 'echo "$A $0"' x=y`, true},
		{"command substitution in arg", `sh -c 'echo "$0"' "$(echo sub)"`, false},
		{"backtick in arg", "sh -c 'echo \"$0\"' \"`echo bt`\"", false},
		{"arithmetic in arg", `sh -c 'echo "$0"' "$((1+2))"`, false},
		{"tilde unquoted", `sh -c 'echo "$0"' ~`, true},
		{"tilde quoted literal", `sh -c 'echo "$0"' "~"`, true},
		{"unicode arg", `sh -c 'echo "$0"' "żółć ✓"`, true},
		{"empty arg", `sh -c 'echo "[$0]"' ""`, true},

		// exit status
		{"exit 0", `sh -c 'exit 0'`, true},
		{"exit 3", `sh -c 'exit 3'`, true},
		{"exit 3 with env", `PG="x" sh -c 'exit 3'`, true},
		{"exit 127 not found", `no-such-binary-xyz >/dev/null 2>&1`, false},
		{"stderr and stdout", `sh -c 'echo out; echo err >&2'`, true},
		{"stderr merged", `sh -c 'echo out; echo err >&2' 2>&1`, true},
		{"stderr only", `ls /no/such/path`, true},

		// lists, pipes, sequencing
		{"and then", `sh -c 'echo a' && sh -c 'echo b'`, false},            // unknown programs in a list stay as they are (payoff gate)
		{"and fails", `sh -c 'exit 2' && sh -c 'echo skipped'`, false},     // unknown programs in a list stay as they are (payoff gate)
		{"semicolon continues", `sh -c 'exit 2'; sh -c 'echo ran'`, false}, // unknown programs in a list stay as they are (payoff gate)
		{"cd and env", `cd sub && PG="v" pwd`, false},
		{"cd and env command", `cd sub && PG="v" sh -c 'pwd | tr / " "'`, false}, // unknown programs in a list stay as they are (payoff gate)
		{"cd then command", `cd sub && ls -a`, true},
		{"env in every segment", `A="1 1" sh -c 'echo $A'; B='2 2' sh -c 'echo $B' && C=3 sh -c 'echo $C'`, false}, // unknown programs in a list stay as they are (payoff gate)
		{"quiet between", `sh -c 'echo a'; echo mid; sh -c 'echo b'`, false},                                       // unknown programs in a list stay as they are (payoff gate)
		{"or list untouched", `sh -c 'exit 1' || sh -c 'echo fallback'`, false},
		{"head pipe", `PG="v" ls -a | head -3`, true},
		{"tail pipe", `ls -a | tail -n 2`, true},
		{"head and tail", `ls -a | head -4 | tail -2`, true},
		{"head after quoted env", `A="x y" sh -c 'seq 1 20; echo "$A"' | tail -2`, true},
		{"quoted pipe in arg", `sh -c 'echo "$0"' "a|b"`, true},
		{"quoted && in arg", `sh -c 'echo "$0"' "a && b"`, true},
		{"quoted semicolon in arg", `sh -c 'echo "$0"' "a; b"`, true},
		{"quoted semicolon single", `sh -c 'echo "$0"' 'a; b'`, true},
		{"env in every known segment", `A="1 1" ls sub; B='2 2' ls a.txt && C=3 ls sub`, true},
		{"env only then known command", `PG="a b"; ls sub`, true},
		{"cd and env known command", `cd sub && PG="v" ls -a`, true},
		{"and then known", `ls sub && ls a.txt`, true},
		{"and fails known", `ls nope-xyz && ls sub`, true},
		{"semicolon continues known", `ls nope-xyz; ls sub`, true},
		{"redirect untouched", `sh -c 'echo hi' > out.txt; cat out.txt`, false},
		{"redirect segment verbatim in a mixed line", `ls a.txt > out.txt; ls out.txt`, true},
		{"redirect segment after a wrapped one", `ls sub && ls a.txt > out2.txt; cat out2.txt`, true},
		{"stderr redirect segment in a mixed line", `ls /no/such/path 2>/dev/null; ls sub`, true},
		{"stdin redirect untouched", `cat < a.txt`, false},
		{"subshell untouched", `sh -c 'echo "$0"' "$(cat a.txt)"`, false},
		{"substitution segment in a mixed line", `echo "$(cat a.txt)"; ls sub`, true},
		{"substitution in a work segment", `ls sub && ls "$(echo a.txt)"`, true},
		{"backtick segment in a mixed line", "echo `cat a.txt`; ls sub", true},
		{"semicolon inside substitution", `echo $(echo a; echo b); ls sub`, true},
		{"semicolon and and inside nested substitution", `echo $(echo $(echo a; echo b) && echo c); ls a.txt`, true},
		{"escaped semicolon", `echo a\;b; ls sub`, true},
		{"escaped and", `echo a\&\&b; ls sub`, true},
		{"find exec escaped semicolon", `find . -name a.txt -exec echo {} \; ; ls sub`, true},
		{"find exec plus", `find . -name a.txt -exec echo {} + && ls sub`, true},
		{"multi-line list", "ls sub\nls a.txt\nls nope-xyz", true},
		{"multi-line with blank lines", "ls sub\n\nls a.txt\n", true},
		{"multi-line with a redirect line", "ls sub > out3.txt\ncat out3.txt\nls a.txt", true},
		{"quoted newline stays in a segment", "ls sub; echo \"a\nb\"; ls a.txt", true},
		{"backslash newline continues", "ls \\\n sub; ls a.txt", true},
		{"or list in a mixed line", `ls sub; ls a.txt || ls sub`, false},
		{"background in a mixed line", `ls sub; sh -c 'echo bg' & wait`, false},
		{"trailing background", `ls sub; ls a.txt &`, false},
		{"for loop in a mixed line", `ls sub; for f in a.txt; do cat $f; done`, false},
		{"for loop over lines", "for f in a.txt\ndo\n ls $f\ndone", false},
		{"if in a mixed line", `ls sub; if [ -f a.txt ]; then cat a.txt; fi`, false},
		{"group in a mixed line", `ls sub; { ls a.txt; ls sub; }`, false},
		{"subshell group", `(cd sub; ls)`, false},
		{"subshell in a mixed line", `ls sub; (cd sub && ls)`, false},
		{"unbalanced substitution", `ls sub; echo $(echo a`, false},
		{"unbalanced backtick", "ls sub; echo `echo a", false},
		{"heredoc", "cat <<EOF\nx; y\nEOF\nls sub", false},
		{"heredoc quoted marker", "cat <<'EOF'\n$HOME\nEOF\nls sub", false},
		{"heredoc dash", "cat <<-EOF\n\tx\nEOF\nls sub", false},
		{"here-string", `cat <<< "x"; ls sub`, false},
		{"pipefail line", `set -o pipefail; ls a.txt | cat; ls sub`, false},
		{"unterminated quote in a later segment", `ls sub; echo "oops`, false},
		{"payoff gate leaves unknown programs", `printenv REPLY; ls sub`, true},
		{"payoff gate all unknown", `printenv REPLY; printenv HOME`, false},
		{"background untouched", `sh -c 'echo bg' & wait`, false},
		{"for loop untouched", `for f in a.txt; do cat $f; done`, false},
		{"if untouched", `if [ -f a.txt ]; then cat a.txt; fi`, false},
		{"group untouched", `{ cat a.txt; cat a.txt; }`, false},
		{"multi-line unknown programs untouched", "cat a.txt\ncat a.txt", false},
		{"env only", `PG="a b"`, false},
		{"env only then command", `PG="a b"; sh -c 'echo "[$PG]"'`, false}, // unknown programs in a list stay as they are (payoff gate)
		{"unterminated quote left alone", `PG="oops printenv PG`, false},
		{"quiet command", `echo hi`, false},
		{"quiet with env", `PG="a b" echo hi`, false},
		{"builtin", `cd sub`, false},
		{"sudo-less wrapper env", `env PG="a b" printenv PG`, true},
		{"command builtin", `command ls a.txt`, false},
		{"exec builtin", `exec ls a.txt`, false},
	}
	prefixes := []string{"gascoigne exec", "gascoigne exec -agent cursor -session 'a b' -model m"}
	for _, prefix := range prefixes {
		for _, tt := range tests {
			t.Run(tt.name+" ["+prefix[:min(len(prefix), 20)]+"]", func(t *testing.T) {
				got, ok := wrapCommand(tt.line, prefix)
				if ok != tt.wrapped {
					t.Fatalf("wrapped = %v, want %v: %q → %q", ok, tt.wrapped, tt.line, got)
				}
				if !ok && got != strings.TrimSpace(tt.line) {
					t.Fatalf("untouched line changed: %q → %q", tt.line, got)
				}
				wantOut, wantCode := runShell(t, dir, bin, tt.line)
				gotOut, gotCode := runShell(t, dir, bin, got)
				if gotOut != wantOut || gotCode != wantCode {
					t.Fatalf("rewrite changed behavior\nline:    %s\nrewrite: %s\nwant: %q exit %d\ngot:  %q exit %d",
						tt.line, got, wantOut, wantCode, gotOut, gotCode)
				}
			})
		}
	}
}

// Wrapping lines the hook would rewrite twice (a second hook run, or an
// agent replaying its command) must not change them again.
func TestWrapCommandIdempotent(t *testing.T) {
	for _, line := range []string{
		`PG="a b" mytool x`,
		`cd api && A=1 B='2 2' go test ./...`,
		`go vet ./...; go test ./... | tail -3`,
	} {
		once, ok := wrapCommand(line, "gascoigne exec")
		if !ok {
			t.Fatalf("%q not wrapped", line)
		}
		if twice, ok := wrapCommand(once, "gascoigne exec"); ok {
			t.Errorf("%q wrapped twice: %q", once, twice)
		}
	}
}
