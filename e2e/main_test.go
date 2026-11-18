// Package e2e runs the compiled gascoigne binary the way a user does and checks what it promises:
// output, exit codes, files, and what the ledger remembers between commands.
//
// Scenarios are Go tables, one file per area (see dispatch_test.go for the smallest example).
// To add one, append a Scenario to the group in the matching file. See docs/development.md.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// tools are the binaries built once for the whole run: name, then the package that builds it.
var tools = map[string]string{
	"gascoigne": "./cmd/gascoigne",
	"e2estub":   "./cmd/e2estub",
	"mcpmock":   "./cmd/mcpmock",
}

var (
	repoRoot string                // the checkout the suite runs in; fixtures are read from here
	toolPath = map[string]string{} // tool name -> built binary
)

func TestMain(suite *testing.M) {
	os.Exit(runSuite(suite))
}

func runSuite(suite *testing.M) int {
	if runtime.GOOS == "windows" {
		fmt.Fprintln(os.Stderr, "e2e: skipped on windows, the sandbox needs symlinks and a POSIX environment")
		return 0
	}
	root, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	repoRoot = root
	out, err := os.MkdirTemp("", "gascoigne-e2e-bin-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(out)
	if err := buildTools(out); err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	before := realConfigStamp()
	code := suite.Run()
	if after := realConfigStamp(); after != before {
		fmt.Fprintln(os.Stderr, "e2e: the real gascoigne config changed during the run; a scenario escaped its sandbox")
		return 1
	}
	return code
}

// buildTools compiles every tool once. GASCOIGNE_E2E_BIN points the suite at a prebuilt
// gascoigne, for example a release artifact, instead of building one.
func buildTools(dir string) error {
	for name, pkg := range tools {
		path := filepath.Join(dir, name)
		toolPath[name] = path
		if prebuilt := os.Getenv("GASCOIGNE_E2E_BIN"); name == "gascoigne" && prebuilt != "" {
			toolPath[name] = prebuilt
			continue
		}
		build := exec.Command("go", "build", "-o", path, pkg)
		build.Dir = repoRoot
		build.Env = append(os.Environ(), "CGO_ENABLED=0")
		if out, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("build %s: %v\n%s", pkg, err, out)
		}
	}
	return nil
}

// realConfigStamp is the modification time of the developer's own gascoigne config directory.
// A past test rewrote that file, so the suite proves it never touches it.
func realConfigStamp() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		dir = filepath.Join(home, ".config")
	}
	var stamp string
	for _, path := range []string{filepath.Join(dir, "gascoigne"), filepath.Join(dir, "gascoigne", "config.yaml")} {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		stamp += fmt.Sprintf("%s=%d;", path, info.ModTime().UnixNano())
	}
	return stamp
}
