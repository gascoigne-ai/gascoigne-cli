package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	stepTimeout    = 10 * time.Second
	deadServerAddr = "127.0.0.1:1"
)

// Scenario is one behavior the CLI promises, written as the commands a user types.
// Name is a sentence about the behavior. Steps run in order in one sandbox, and the
// scenario stops at the first step that fails, since later steps depend on it.
type Scenario struct {
	Name   string
	Agents []string          // stand-in agents put on PATH: claude, codex, gemini, opencode, pi
	Files  map[string]string // written into the working dir; "@testdata/..." copies a repo fixture
	Env    []string          // KEY=value for every step, on top of the sandbox environment
	Server bool              // a real background server on a free port, stopped at the end
	Slow   bool              // runs only with GASCOIGNE_E2E=all (make e2e)
	Steps  []Step
}

// Step is one command and what must be true after it ran.
type Step struct {
	Run    string             // "gascoigne exec -- emit out.txt"; ${name} expands saved values and built-ins
	Stdin  string             // literal text, or "@testdata/..." for a repo fixture
	Env    []string           // KEY=value for this step only
	Exit   int                // expected exit code
	Stdout []Check            // all must pass
	Stderr []Check            // all must pass
	Save   map[string]string  // variable name -> regexp with one group, matched over stdout
	Files  map[string][]Check // path under the working dir -> checks over its content
	Gone   []string           // paths (with ${home} etc.) that must not exist after the step
	MCP    []Call             // Run starts an MCP server on stdio; stdout is then its tool names
}

// Built-in variables for Run, Env values and Files keys: ${root} ${home} ${work} ${bin} ${record}
// (the file a stand-in agent writes its start-up record to).

var registry []namedGroup

type namedGroup struct {
	name      string
	scenarios []Scenario
}

// group registers scenarios under a name for TestScenarios. Call it from a package-level
// `var _ = group(...)` in the file for that area, so a new file needs no wiring.
func group(name string, scenarios ...Scenario) []Scenario {
	registry = append(registry, namedGroup{name, scenarios})
	return scenarios
}

func TestScenarios(t *testing.T) {
	for _, entry := range registry {
		t.Run(entry.name, func(t *testing.T) {
			for _, scenario := range entry.scenarios {
				t.Run(scenario.Name, func(t *testing.T) {
					if scenario.Slow && os.Getenv("GASCOIGNE_E2E") != "all" {
						t.Skip("slow scenario: run make e2e, or set GASCOIGNE_E2E=all")
					}
					t.Parallel()
					runScenario(t, scenario)
				})
			}
		})
	}
}

func runScenario(t *testing.T, scenario Scenario) {
	box := newSandbox(t, scenario)
	for number, step := range scenario.Steps {
		result := box.run(t, step)
		if err := box.verify(step, result); err != nil {
			t.Fatalf("%s\nstep %d: %s\n%v\nexit %d\nstdout:\n%s\nstderr:\n%s\nsandbox: %s",
				scenario.Name, number+1, step.Run, err, result.exit, result.stdout, result.stderr, box.root)
		}
	}
}

// sandbox is a private HOME, XDG tree, working dir and bin dir for one scenario.
type sandbox struct {
	root string
	vars map[string]string
	env  []string
}

func newSandbox(t *testing.T, scenario Scenario) *sandbox {
	root, err := os.MkdirTemp("", "gascoigne-e2e-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if !t.Failed() {
			os.RemoveAll(root)
		}
	})
	box := &sandbox{root: root, vars: map[string]string{
		"home":   filepath.Join(root, "home"),
		"record": filepath.Join(root, "agent-record.json"),
		"work":   filepath.Join(root, "work"),
		"root":   root,
		"bin":    filepath.Join(root, "bin"),
	}}
	for _, dir := range []string{box.vars["home"], box.vars["work"], box.vars["bin"]} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	box.link(t, "gascoigne", "gascoigne")
	box.link(t, "emit", "e2estub")
	box.link(t, "mcpmock", "mcpmock")
	for _, agent := range scenario.Agents {
		box.link(t, agent, "e2estub")
	}
	box.env = box.baseEnv()
	if scenario.Server {
		box.useRealServer(t)
	}
	box.env = append(box.env, box.expandAll(scenario.Env)...)
	for path, content := range scenario.Files {
		box.writeFile(t, path, content)
	}
	return box
}

// useRealServer lets the scenario start the background server, on a free port of its own and
// with an idle limit, so a scenario that dies before cleanup leaves no server running for long.
// Cleanup stops the server and waits until its port is closed.
func (box *sandbox) useRealServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	box.env = slices.DeleteFunc(box.env, func(entry string) bool {
		return strings.HasPrefix(entry, "GASCOIGNE_SERVER=") || strings.HasPrefix(entry, "GASCOIGNE_SERVER_ADDR=")
	})
	box.env = append(box.env, "GASCOIGNE_SERVER_ADDR="+addr, "GASCOIGNE_SERVER_IDLE=60s")
	t.Cleanup(func() {
		stop := exec.Command(filepath.Join(box.vars["bin"], "gascoigne"), "server", "stop")
		stop.Dir, stop.Env = box.vars["work"], box.env
		stop.Run()
		for range 50 {
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err != nil {
				return
			}
			conn.Close()
			time.Sleep(100 * time.Millisecond)
		}
		t.Errorf("server on %s still listening after server stop", addr)
	})
}

// baseEnv is the whole environment a step sees. It is built from nothing, never from the
// developer's own, so no variable of theirs changes a result and no path leaks out.
func (box *sandbox) baseEnv() []string {
	home := box.vars["home"]
	return []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"TMPDIR=" + box.vars["work"],
		"PATH=" + box.vars["bin"],
		"CODEX_HOME=",
		"E2E_RECORD=" + box.vars["record"],
		"GASCOIGNE_LEDGER=" + filepath.Join(box.root, "ledger.db"),
		"GASCOIGNE_SERVER=off",
		// `uninstall` and `server` look for a running server whatever GASCOIGNE_SERVER says. Nothing
		// listens on port 1, so no scenario can reach the developer's real server and stop it.
		"GASCOIGNE_SERVER_ADDR=" + deadServerAddr,
		"GASCOIGNE_PROXY=off",
		"GASCOIGNE_RELEASE_CHECK=off",
		"GASCOIGNE_MCP_REVIEW=off",
		"GASCOIGNE_AUTOSYNC=off",
		"NO_COLOR=1",
		"TZ=UTC",
		"LANG=C",
	}
}

// link puts a built tool into the sandbox bin under a persona name. A hard link keeps
// `uninstall` from deleting the shared binary; a copy is the fallback across filesystems.
func (box *sandbox) link(t *testing.T, name, tool string) {
	source, ok := toolPath[tool]
	if !ok {
		t.Fatalf("no built tool %q", tool)
	}
	target := filepath.Join(box.vars["bin"], name)
	if err := os.Link(source, target); err == nil {
		return
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o755); err != nil {
		t.Fatal(err)
	}
}

func (box *sandbox) writeFile(t *testing.T, path, content string) {
	data, err := loadContent(content)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(box.vars["work"], path)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// loadContent returns literal text, or the bytes of a repo fixture for "@path".
func loadContent(content string) ([]byte, error) {
	fixture, ok := strings.CutPrefix(content, "@")
	if !ok {
		return []byte(content), nil
	}
	return os.ReadFile(filepath.Join(repoRoot, fixture))
}

type result struct {
	exit    int
	stdout  string
	stderr  string
	failure error // an MCP session that could not run or a call that went wrong
}

func (box *sandbox) run(t *testing.T, step Step) result {
	words, err := box.words(step.Run)
	if err != nil {
		t.Fatalf("step %q: %v", step.Run, err)
	}
	if len(step.MCP) > 0 {
		return box.runMCP(t, step, words)
	}
	ctx, cancel := context.WithTimeout(t.Context(), stepTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(box.vars["bin"], words[0]), words[1:]...)
	cmd.Dir = box.vars["work"]
	cmd.Env = append(append([]string{}, box.env...), box.expandAll(step.Env)...)
	cmd.WaitDelay = time.Second
	if step.Stdin != "" {
		stdin, err := loadContent(step.Stdin)
		if err != nil {
			t.Fatal(err)
		}
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case errors.As(err, &exitErr):
		return result{exit: exitErr.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
	case err != nil:
		t.Fatalf("step %q: %v\nstderr:\n%s", step.Run, err, stderr.String())
	}
	return result{stdout: stdout.String(), stderr: stderr.String()}
}

// verify checks the exit code, both streams and files, then saves variables for later steps.
func (box *sandbox) verify(step Step, got result) error {
	if got.failure != nil {
		return got.failure
	}
	if got.exit != step.Exit {
		return fmt.Errorf("exit code %d, want %d", got.exit, step.Exit)
	}
	for _, check := range step.Stdout {
		if err := check(box, got.stdout); err != nil {
			return fmt.Errorf("stdout: %w", err)
		}
	}
	for _, check := range step.Stderr {
		if err := check(box, got.stderr); err != nil {
			return fmt.Errorf("stderr: %w", err)
		}
	}
	for path, checks := range step.Files {
		data, err := os.ReadFile(box.expand(path))
		if err != nil {
			return fmt.Errorf("file %s: %w", path, err)
		}
		for _, check := range checks {
			if err := check(box, string(data)); err != nil {
				return fmt.Errorf("file %s: %w", path, err)
			}
		}
	}
	for _, path := range step.Gone {
		if _, err := os.Stat(box.expand(path)); err == nil {
			return fmt.Errorf("%s exists, want it gone", path)
		}
	}
	return box.save(step.Save, got.stdout)
}

func (box *sandbox) save(pairs map[string]string, stdout string) error {
	for name, pattern := range pairs {
		match := regexp.MustCompile(pattern).FindStringSubmatch(stdout)
		if len(match) != 2 {
			return fmt.Errorf("save %s: %q has no match in stdout", name, pattern)
		}
		box.vars[name] = match[1]
	}
	return nil
}

// words splits a Run line like a shell would for plain words and quotes (no escapes,
// no pipes), then expands ${name} in each word so a saved value with spaces stays one word.
func (box *sandbox) words(line string) ([]string, error) {
	var words []string
	for _, field := range splitQuoted(line) {
		words = append(words, box.expand(field))
	}
	if len(words) == 0 {
		return nil, errors.New("empty Run")
	}
	return words, nil
}

// splitQuoted splits on spaces; a quoted run joins with its neighbours into one word.
func splitQuoted(line string) []string {
	var words []string
	var word strings.Builder
	var quote rune
	inWord := false
	for _, char := range line {
		switch {
		case quote != 0 && char == quote:
			quote = 0
		case quote != 0:
			word.WriteRune(char)
		case char == '\'' || char == '"':
			quote, inWord = char, true
		case char == ' ' || char == '\t':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(char)
			inWord = true
		}
	}
	if inWord {
		words = append(words, word.String())
	}
	return words
}

func (box *sandbox) expandAll(texts []string) []string {
	expanded := make([]string, len(texts))
	for i, text := range texts {
		expanded[i] = box.expand(text)
	}
	return expanded
}

// expand replaces ${name}; an unknown name stays visible so the failure points at it.
func (box *sandbox) expand(text string) string {
	for name, value := range box.vars {
		text = strings.ReplaceAll(text, "${"+name+"}", value)
	}
	return text
}
