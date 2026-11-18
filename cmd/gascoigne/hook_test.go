package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWrapCommand(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"known tool", "go test ./...", "gascoigne exec -- go test ./..."},
		{"after cd", "cd api && go test ./...", "cd api && gascoigne exec -- go test ./..."},
		{"env kept outside", "CGO_ENABLED=0 go build ./...", "CGO_ENABLED=0 gascoigne exec -- go build ./..."},
		{"stderr merge", "npm install 2>&1", "gascoigne exec -- npm install 2>&1"},
		{"quoted args", `git log --format="%h %s"`, `gascoigne exec -- git log --format="%h %s"`},
		{"quiet command", "echo hi", ""},
		{"unknown tool", "./scripts/report.py --json", "gascoigne exec -- ./scripts/report.py --json"},
		{"custom binary", "mytool list", "gascoigne exec -- mytool list"},
		{"repl", "python3", ""},
		{"script", "python3 gen.py", "gascoigne exec -- python3 gen.py"},
		{"quiet behind sudo", "sudo rm -rf build", ""},
		{"env only", "FOO=1", ""},
		{"time keyword", "time go test ./...", ""},
		{"quoted env", `PG="aaaa" pbbb "hjj"`, `PG="aaaa" gascoigne exec -- pbbb "hjj"`},
		{"quoted env with space", `PG='a b' FOO=1 pbbb x`, `PG='a b' FOO=1 gascoigne exec -- pbbb x`},
		{"quoted env only", `PG="a b"`, ""},
		{"quiet after env", "A=1 echo hi", ""},
		{"flag with =", "x --format=y", "gascoigne exec -- x --format=y"},
		{"quoted env after cd", `cd api && PG="a" git status`, `cd api && PG="a" gascoigne exec -- git status`},
		{"builtin", "cd api", ""},
		{"tail pipe", "go test ./... | tail -5", "gascoigne exec -- go test ./... | tail -5"},
		{"head pipe", "git log -n 50 2>&1 | head -n 20", "gascoigne exec -attr hint=top -- git log -n 50 2>&1 | head -n 20"},
		{"head and tail", "cd api && go test ./... | head -40 | tail -5", "cd api && gascoigne exec -attr hint=top -- go test ./... | head -40 | tail -5"},
		{"grep pipe", "go test ./... | grep FAIL", "go test ./... | grep FAIL | gascoigne run -attr cmd='grep FAIL'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"followed tail pipe", "go test ./... | tail -f", ""},
		{"head with file", "go test ./... | head out.txt", "go test ./... | head out.txt | gascoigne run -attr cmd='head out.txt'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"or list", "go test ./... || true", ""},
		{"sequence", "go vet ./...; go test ./...", "gascoigne exec -- go vet ./...; gascoigne exec -- go test ./..."},
		{"every command", "ls docs; cat a.md && git log -5", "gascoigne exec -- ls docs; gascoigne exec -- cat a.md && gascoigne exec -- git log -5"},
		{"hint first only before head", "git status && git log | head -5", "gascoigne exec -- git status && gascoigne exec -attr hint=top -- git log | head -5"},
		{"streaming segment skipped", "npm run dev; go test ./...", "npm run dev; gascoigne exec -- go test ./..."},
		{"tsc watch", "npx tsc -w -p .", "gascoigne exec -watch -- npx tsc -w -p ."},
		{"webpack dev server", "webpack serve --mode development", "gascoigne exec -watch -- webpack serve --mode development"},
		{"vite build watch", "vite build --watch", "gascoigne exec -watch -- vite build --watch"},
		{"vite dev server", "npx vite --port 5173", ""},
		{"watch flag of another family", "jest --watch", ""},
		{"quiet segments kept", "echo start; go test ./...; echo end", "echo start; gascoigne exec -- go test ./...; echo end"},
		{"conditional", "if [ -f go.mod ]; then go test ./...; fi", ""},
		{"while loop", "while true; do go test ./...; done", ""},
		{"grouping", "{ go vet ./...; go test ./...; }", ""},
		{"mixed lists", "cd api; make gen && go test ./...", "cd api; gascoigne exec -- make gen && gascoigne exec -- go test ./..."},
		{"quoted ;", `git log --format="%h;%s"`, `gascoigne exec -- git log --format="%h;%s"`},
		{"loop", "for d in a b; do go test ./$d; done", ""},
		{"path named gascoigne", "cd ~/src/gascoigne && go test ./...", "cd ~/src/gascoigne && gascoigne exec -- go test ./..."},
		{"local binary", "./bin/gascoigne gain", ""},
		{"redirect", "go test ./... > out.txt", ""},
		{"subshell", "kubectl get pods -n $(cat ns)", ""},
		{"background", "go test ./... &", ""},
		{"already wrapped", "gascoigne exec -- go test ./...", ""},
		{"raw requested", "GASCOIGNE_RAW=1 go test ./...", ""},
		{"quoted &&", `echo "a && go test ./..."`, ""},
		{"followed logs", "kubectl logs -f api", ""},
		{"watch", "kubectl get pods --watch", ""},
		{"dev server", "npm run dev", ""},
		{"compose up", "docker compose up", ""},
		{"multiline", "go test ./...\ngo vet ./...", "gascoigne exec -- go test ./...\ngascoigne exec -- go vet ./..."},

		// Operators inside quotes are literal; a head or tail ends any stage of a list, not only the last.
		{"quoted pipe of a grep pattern", `grep -n 'a\|b' x.go | head -20`, `gascoigne exec -attr hint=top -- grep -n 'a\|b' x.go | head -20`},
		{"quoted arrow and angle brackets", `grep -n 'a->b' x.go; sed -n '/<h1>/p' index.html`, `gascoigne exec -- grep -n 'a->b' x.go; gascoigne exec -- sed -n '/<h1>/p' index.html`},
		{"double-quoted ampersand", `grep -n "a & b" x.go`, `gascoigne exec -- grep -n "a & b" x.go`},
		{"escaped pipe outside quotes", `grep a\|b x.go`, `gascoigne exec -- grep a\|b x.go`},
		{"head before the end of a list", "sed -n 1,60p a.go | head -40; grep -n x b.go | head -5", "gascoigne exec -attr hint=top -- sed -n 1,60p a.go | head -40; gascoigne exec -attr hint=top -- grep -n x b.go | head -5"},
		{"head in the first stage only", "git show HEAD:f | head -40; sed -n 1,60p g.sh", "gascoigne exec -attr hint=top -- git show HEAD:f | head -40; gascoigne exec -- sed -n 1,60p g.sh"},
		{"tail in the middle of a && list", "go test ./... | tail -5 && git status", "gascoigne exec -- go test ./... | tail -5 && gascoigne exec -- git status"},
		{"stderr merge before the end", "cat a.json 2>&1; ls docs", "gascoigne exec -- cat a.json 2>&1; gascoigne exec -- ls docs"},
		{"pipeline ends a list", "ls | head -3; go test ./... | grep FAIL", "gascoigne exec -attr hint=top -- ls | head -3; go test ./... | grep FAIL | gascoigne run -attr cmd='grep FAIL'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"pipeline in the middle of a list", "go test ./... | grep FAIL; ls", "{ go test ./... | grep FAIL | gascoigne run -attr cmd='grep FAIL'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); }; gascoigne exec -- ls"},
		{"quoted pipe then a real one", `grep 'a|b' x.go | wc -l`, `grep 'a|b' x.go | wc -l | gascoigne run -attr cmd='wc -l'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})`},
		{"three stages after a list", "cd api && cat a.json | jq .items | grep x", "cd api && { cat a.json | jq .items | grep x | gascoigne run -attr cmd='grep x'; (exit ${PIPESTATUS[2]:-${pipestatus[3]}}); }"},
		{"pipe with env on the last stage", "ls | A=1 grep x", "ls | A=1 grep x | gascoigne run -attr cmd='grep x'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"pipe into a redirect", "ls | grep x > out.txt", ""},
		{"pipe with substitution", "ls | grep $(cat p)", ""},
		{"pipe with pipefail", "set -o pipefail; ls | grep x", ""},
		{"streaming first stage", "kubectl logs -f api | grep err", ""},
		{"pipe into a loop", "ls | while read f; do echo $f; done", ""},
		{"pipe or list", "ls | grep x || true", ""},
		{"empty stage", "ls | | grep x", ""},
		{"quiet first stage", "echo hi | grep h", ""},
		{"substitution inside double quotes still runs", `grep "$(cat p)" x.go`, ""},
		{"backtick inside double quotes", "grep \"`cat p`\" x.go", ""},
		{"redirect after quotes", `grep 'a' x.go > out.txt`, ""},
		{"background after quotes", `grep 'a' x.go &`, ""},
		{"or list with quotes", `grep 'a' x.go || true`, ""},
		{"unterminated quote", `grep 'a x.go | head`, ""},

		// Compound lines are rewritten per segment: what cannot be wrapped stays verbatim, the rest is wrapped.
		{"redirect segment verbatim", "go test ./... > out.txt; go vet ./...", "go test ./... > out.txt; gascoigne exec -- go vet ./..."},
		{"redirect segment after", "go vet ./... && go test ./... > out.txt", "gascoigne exec -- go vet ./... && go test ./... > out.txt"},
		{"substitution segment verbatim", `echo "$(date)"; go vet ./...`, `echo "$(date)"; gascoigne exec -- go vet ./...`},
		{"substitution in a work segment", "git status && go test $(go list ./...)", "gascoigne exec -- git status && go test $(go list ./...)"},
		{"semicolon inside substitution", "echo $(echo a; echo b); go vet ./...", "echo $(echo a; echo b); gascoigne exec -- go vet ./..."},
		{"backtick holds a list", "echo `echo a; echo b`; go vet ./...", "echo `echo a; echo b`; gascoigne exec -- go vet ./..."},
		{"escaped semicolon", `find . -name x -exec cat {} \; ; ls`, `gascoigne exec -- find . -name x -exec cat {} \;; gascoigne exec -- ls`},
		{"escaped ampersands", `echo a\&\&b; go vet ./...`, `echo a\&\&b; gascoigne exec -- go vet ./...`},
		{"multi-line list", "git status\nls docs\ngo vet ./...", "gascoigne exec -- git status\ngascoigne exec -- ls docs\ngascoigne exec -- go vet ./..."},
		{"blank line and trailing newline", "git status\n\nls docs\n", "gascoigne exec -- git status\n\ngascoigne exec -- ls docs"},
		{"quoted newline stays in its segment", "grep -n \"a\nb\" x.go; ls", "gascoigne exec -- grep -n \"a\nb\" x.go; gascoigne exec -- ls"},
		{"backslash newline continues", "go test \\\n ./...; git status", "gascoigne exec -- go test \\\n ./...; gascoigne exec -- git status"},
		{"redirect line in a script", "ls > files.txt\ngo vet ./...", "ls > files.txt\ngascoigne exec -- go vet ./..."},
		{"or list bails", "go vet ./... || go test ./...", ""},
		{"or list after a list", "git status; go vet ./... || true", ""},
		{"background bails", "go vet ./... & go test ./...", ""},
		{"trailing background bails", "git status; go test ./... &", ""},
		{"heredoc bails", "cat <<EOF\nx\nEOF\ngo vet ./...", ""},
		{"quoted heredoc bails", "cat <<'EOF'\nx\nEOF\ngo vet ./...", ""},
		{"here-string bails", "go vet ./... <<< x; ls", ""},
		{"heredoc marker in quotes is text", `echo "<<EOF"; go vet ./...`, `echo "<<EOF"; gascoigne exec -- go vet ./...`},
		{"pipefail bails", "set -o pipefail; go vet ./...", ""},
		{"subshell bails", "(cd api; go test ./...); go vet ./...", ""},
		{"bare paren bails", "go vet ./...; (cd api && go test ./...)", ""},
		{"loop over lines bails", "for d in a b\ndo\n go test ./$d\ndone", ""},
		{"if over lines bails", "if true\nthen go vet ./...\nfi", ""},
		{"case bails", "case x in x) go vet ./... ;; esac", ""},
		{"group after a list bails", "git status; { go vet ./...; }", ""},
		{"unbalanced substitution bails", "go vet ./...; echo $(echo a", ""},
		{"unbalanced backtick bails", "go vet ./...; echo `echo a", ""},
		{"unterminated quote in a later segment bails", "go vet ./...; echo 'a", ""},
		{"own command later bails", "go vet ./...\ngascoigne gain", ""},

		// Pipelines anywhere in a list: the last one after ; keeps the plain form, any other one is braced so
		// PIPESTATUS and the status are read right after it.
		{"pipeline before and", "seq 1 5 | grep 9 && echo skipped", "{ seq 1 5 | grep 9 | gascoigne run -attr cmd='grep 9'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); } && echo skipped"},
		{"pipeline before semicolon", "seq 1 5 | grep 9; echo $?", "{ seq 1 5 | grep 9 | gascoigne run -attr cmd='grep 9'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); }; echo $?"},
		{"pipeline after and is braced", "git status && seq 1 5 | grep 3", "gascoigne exec -- git status && { seq 1 5 | grep 3 | gascoigne run -attr cmd='grep 3'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); }"},
		{"pipeline last after semicolon is plain", "git status; seq 1 5 | grep 3", "gascoigne exec -- git status; seq 1 5 | grep 3 | gascoigne run -attr cmd='grep 3'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"pipeline last after newline is plain", "git status\nseq 1 5 | grep 3", "gascoigne exec -- git status\nseq 1 5 | grep 3 | gascoigne run -attr cmd='grep 3'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
		{"two pipelines", "seq 1 5 | grep 3 && seq 1 5 | grep 4", "{ seq 1 5 | grep 3 | gascoigne run -attr cmd='grep 3'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); } && { seq 1 5 | grep 4 | gascoigne run -attr cmd='grep 4'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}}); }"},
		{"pipeline with redirect stays verbatim", "seq 1 5 | grep 3 > out.txt; git status", "seq 1 5 | grep 3 > out.txt; gascoigne exec -- git status"},
		{"pipeline of quiet stage stays verbatim", "echo a | grep a; git status", "echo a | grep a; gascoigne exec -- git status"},

		// Payoff gate: in a list only commands of a known dialect are worth a hint.
		{"unknown program in a list stays", "mytool x; git status", "mytool x; gascoigne exec -- git status"},
		{"all unknown in a list is untouched", "mytool x; mytool y", ""},
		{"unknown after cd is untouched", "cd api && mytool x", ""},
		{"unknown single command still wrapped", "mytool x", "gascoigne exec -- mytool x"},
		{"unknown pipeline end in a list stays", "seq 1 5 | sort -u && git status", "seq 1 5 | sort -u && gascoigne exec -- git status"},
		{"unknown pipeline end alone still wrapped", "seq 1 5 | sort -u", "seq 1 5 | sort -u | gascoigne run -attr cmd='sort -u'; (exit ${PIPESTATUS[1]:-${pipestatus[2]}})"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := wrapCommand(tt.in, "gascoigne exec")
			if want := tt.want != ""; ok != want {
				t.Fatalf("ok = %v, got %q", ok, got)
			}
			if ok && got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// segments is the lexer under wrapCommand: it cuts only at separators that run at the top level of the line.
func TestSegments(t *testing.T) {
	tests := []struct {
		name, in string
		parts    []string
		seps     []string
		ok       bool
	}{
		{"one command", "ls -a", []string{"ls -a"}, nil, true},
		{"semicolon and and", "a; b && c", []string{"a", "b", "c"}, []string{";", "&&"}, true},
		{"newline", "a\nb", []string{"a", "b"}, []string{"\n"}, true},
		{"quoted separators", `a "x; y && z\nw" 'p; q'`, []string{`a "x; y && z\nw" 'p; q'`}, nil, true},
		{"escaped separators", `a \; b \&\& c`, []string{`a \; b \&\& c`}, nil, true},
		{"backslash newline continues", "a \\\nb; c", []string{"a \\\nb", "c"}, []string{";"}, true},
		{"escaped quote in double quotes", `a "x\"; y"; b`, []string{`a "x\"; y"`, "b"}, []string{";"}, true},
		{"substitution", "a $(b; c && d); e", []string{"a $(b; c && d)", "e"}, []string{";"}, true},
		{"nested substitution", "a $(b $(c; d); e)", []string{"a $(b $(c; d); e)"}, nil, true},
		{"quotes inside substitution", `a $(b ")" ';'); e`, []string{`a $(b ")" ';')`, "e"}, []string{";"}, true},
		{"substitution inside double quotes", `a "$(b; c)"; d`, []string{`a "$(b; c)"`, "d"}, []string{";"}, true},
		{"arithmetic", "a $((1 + 2)); b", []string{"a $((1 + 2))", "b"}, []string{";"}, true},
		{"backticks", "a `b; c`; d", []string{"a `b; c`", "d"}, []string{";"}, true},
		{"parameter expansion", "a ${x:-p;q}; b", []string{"a ${x:-p;q}", "b"}, []string{";"}, true},
		{"redirect ampersands", "a 2>&1; b &>f; c >&2", []string{"a 2>&1", "b &>f", "c >&2"}, []string{";", ";"}, true},
		{"empty segments are kept", "a;\n\nb;", []string{"a", "", "", "b", ""}, []string{";", "\n", "\n", ";"}, true},
		{"or list", "a || b", nil, nil, false},
		{"background", "a & b", nil, nil, false},
		{"pipe ampersand", "a |& b", nil, nil, false},
		{"heredoc", "a <<EOF", nil, nil, false},
		{"here-string", "a <<< x", nil, nil, false},
		{"subshell", "(a; b)", nil, nil, false},
		{"closing paren", "a)", nil, nil, false},
		{"open substitution", "a $(b; c", nil, nil, false},
		{"open backtick", "a `b; c", nil, nil, false},
		{"open double quote", `a "b; c`, nil, nil, false},
		{"open single quote", "a 'b; c", nil, nil, false},
		{"open expansion", "a ${b; c", nil, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts, seps, ok := segments(tt.in)
			if ok != tt.ok || ok && (!slices.Equal(parts, tt.parts) || !slices.Equal(seps, tt.seps)) {
				t.Fatalf("segments(%q) = %q %q %v, want %q %q %v", tt.in, parts, seps, ok, tt.parts, tt.seps, tt.ok)
			}
		})
	}
}

func TestRewriteReplies(t *testing.T) {
	in := hookInput{ToolInput: map[string]any{"command": "go test ./...", "description": "run tests"}}
	tests := []struct {
		agent, want string
	}{
		{"claude", `{"hookSpecificOutput":{"hookEventName":"PreToolUse","updatedInput":{"command":"gascoigne exec -attr query=tests -- go test ./...","description":"run tests"}}}`},
		{"cursor", `{"permission":"allow","updated_input":{"command":"gascoigne exec -attr query=tests -- go test ./...","description":"run tests"}}`},
		{"pi", `{"command":"gascoigne exec -attr query=tests -- go test ./..."}`},
		{"opencode", `{"command":"gascoigne exec -attr query=tests -- go test ./..."}`},
		{"gemini", `{"hookSpecificOutput":{"tool_input":{"command":"gascoigne exec -attr query=tests -- go test ./..."}}}`},
	}
	for _, tt := range tests {
		t.Run(tt.agent, func(t *testing.T) {
			reply, err := agents[tt.agent].hook(t.Context(), &common{}, in, "gascoigne exec")
			if err != nil {
				t.Fatal(err)
			}
			if got := compactJSON(reply); got != tt.want {
				t.Fatalf("got %s\nwant %s", got, tt.want)
			}
		})
	}
}

func TestRewriteLeavesAlone(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]any
	}{
		{"unknown tool", map[string]any{"command": "echo hi"}},
		{"background", map[string]any{"command": "go test ./...", "run_in_background": true}},
		{"no command", map[string]any{"file_path": "/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reply, _ := agents["claude"].hook(t.Context(), &common{}, hookInput{ToolInput: tt.input}, "gascoigne exec")
			if reply != nil {
				t.Fatalf("claude got %v", reply)
			}
			reply, _ = agents["cursor"].hook(t.Context(), &common{}, hookInput{ToolInput: tt.input}, "gascoigne exec")
			if got := compactJSON(reply); got != `{"permission":"allow"}` {
				t.Fatalf("cursor got %s", got)
			}
		})
	}
}

func TestShellOutput(t *testing.T) {
	tests := []struct {
		raw, text string
		code      int
		ok        bool
	}{
		{`"out\n"`, "out\n", 0, true},
		{`{"output":"FAIL\n","exit_code":1}`, "FAIL\n", 1, true},
		{`{"other":1}`, "", 0, false},
		{``, "", 0, false},
	}
	for _, tt := range tests {
		text, code, ok := shellOutput(json.RawMessage(tt.raw))
		if text != tt.text || code != tt.code || ok != tt.ok {
			t.Errorf("%s: got %q %d %v", tt.raw, text, code, ok)
		}
	}
}

func TestCodexOptimizesOutput(t *testing.T) {
	t.Setenv("GASCOIGNE_LEDGER", filepath.Join(t.TempDir(), "ledger.db"))
	t.Setenv("GASCOIGNE_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fixture, err := os.ReadFile("../../testdata/eval/go/test-fail-v.txt")
	if err != nil {
		t.Fatal(err)
	}
	resp, _ := json.Marshal(map[string]any{"output": string(fixture), "exit_code": 1})
	in := hookInput{ToolInput: map[string]any{"command": "go test -v ./..."}, ToolResponse: resp}
	c := &common{attrs: attrFlag{}}
	reply, err := agents["codex"].hook(t.Context(), c, in, "gascoigne")
	if err != nil {
		t.Fatal(err)
	}
	m := decodedReply(t, reply)
	reason, _ := m["reason"].(string)
	if m["decision"] != "block" || !strings.HasPrefix(reason, "exit code 1\n") || !strings.Contains(reason, "[raw:") {
		t.Fatalf("got %v", reply)
	}
	if len(reason) >= len(fixture) {
		t.Fatalf("not smaller: %d >= %d", len(reason), len(fixture))
	}

	in.ToolInput["command"] = "gascoigne exec -- go test -v ./..."
	if reply, _ := agents["codex"].hook(t.Context(), c, in, "gascoigne"); reply != nil {
		t.Fatalf("wrapped command optimized twice: %v", reply)
	}
}

func TestExecPrefix(t *testing.T) {
	tests := []struct {
		name, envAgent, envSession, hookSession, model, want string
	}{
		{"wrapped: env says it all", "claude", "claude-20261010", "uuid-1", "", "gascoigne exec"},
		{"installed for good: hook names the agent", "", "", "uuid-1", "", "gascoigne exec -agent cursor -session uuid-1"},
		{"no session from the hook", "", "", "", "", "gascoigne exec -agent cursor"},
		{"odd session id quoted", "", "", "a b", "", "gascoigne exec -agent cursor -session 'a b'"},
		{"model from the transcript", "claude", "s", "", "claude-opus-5-5", "gascoigne exec -model claude-opus-5-5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GASCOIGNE_AGENT", tt.envAgent)
			t.Setenv("GASCOIGNE_SESSION", tt.envSession)
			if got := execPrefix("gascoigne", "cursor", tt.hookSession, tt.model, 0); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// A large context makes every hook result costlier to re-read, so budgets
// shrink; truncate's head and tail must stay below its max or the config is rejected.
func TestExecPrefixTightBudgets(t *testing.T) {
	t.Setenv("GASCOIGNE_AGENT", "claude")
	t.Setenv("GASCOIGNE_SESSION", "s")
	tests := []struct {
		name    string
		context int
		tight   bool
	}{
		{"unknown context", 0, false},
		{"small context", tightContextTokens - 1, false},
		{"large context", tightContextTokens, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := execPrefix("gascoigne", "claude", "", "", tt.context)
			if want := "gascoigne exec"; !tt.tight && got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
			if tt.tight && got != "gascoigne exec"+tightBudgets {
				t.Fatalf("got %q", got)
			}
		})
	}
	if !strings.Contains(tightBudgets, "max_tokens=4000") {
		t.Fatal("truncate max_tokens not tightened")
	}
}

func TestRewriteBackgroundWatch(t *testing.T) {
	in := hookInput{ToolInput: map[string]any{"command": "npx tsc -w", "run_in_background": true}}
	reply, err := agents["claude"].hook(t.Context(), &common{}, in, "gascoigne exec")
	if err != nil {
		t.Fatal(err)
	}
	if got := compactJSON(reply); !strings.Contains(got, `"command":"gascoigne exec -watch -- npx tsc -w"`) {
		t.Fatalf("got %s", got)
	}
}

func TestTranscriptModel(t *testing.T) {
	file := filepath.Join(t.TempDir(), "t.jsonl")
	lines := `{"type":"user","message":{"content":"say \"model\":\"gpt-5\" please"}}
{"type":"assistant","message":{"model":"claude-sonnet-5-5","id":"m1"}}
{"type":"assistant","message":{"model":"claude-opus-5-5","id":"m2"}}
{"type":"user","message":{"content":"x"}}
`
	if err := os.WriteFile(file, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := transcriptModel(file); got != "claude-opus-5-5" {
		t.Fatalf("got %q", got)
	}
	if got := transcriptModel(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Fatalf("missing file: %q", got)
	}
}

// Every agent that rewrites commands shares wrapCommand: quoted pipes are
// literal, a multi-line command is wrapped per line, and an or list stays as it is.
func TestRewriteSharedWrapping(t *testing.T) {
	tests := []struct {
		name, cmd, want string
	}{
		{"quoted pipe", `grep 'a\|b' f.txt | head -5`, `gascoigne exec -attr hint=top -- grep 'a\|b' f.txt | head -5`},
		{"two commands", "go build ./... && go vet ./... | tail -3", "gascoigne exec -- go build ./... && gascoigne exec -- go vet ./... | tail -3"},
		{"multi-line", "go build ./...\ngo vet ./...", "gascoigne exec -- go build ./...\ngascoigne exec -- go vet ./..."},
		{"or list", "go build ./... || go vet ./...", ""},
	}
	for _, name := range []string{"claude", "cursor", "gemini", "opencode", "pi"} {
		for _, tt := range tests {
			t.Run(name+" "+tt.name, func(t *testing.T) {
				in := hookInput{ToolInput: map[string]any{"command": tt.cmd}}
				reply, err := agents[name].hook(t.Context(), &common{}, in, "gascoigne exec")
				if err != nil {
					t.Fatal(err)
				}
				got := compactJSON(reply)
				if tt.want == "" && strings.Contains(got, "gascoigne exec") || tt.want != "" && !strings.Contains(got, jsonString(tt.want)) {
					t.Fatalf("got %s\nwant %q", got, tt.want)
				}
			})
		}
	}
}

func jsonString(text string) string {
	b, _ := json.Marshal(text)
	return strings.Trim(string(b), `"`)
}
