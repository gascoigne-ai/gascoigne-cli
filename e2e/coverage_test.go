package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Drift guards: the commands the binary lists, the commands docs/cli.md lists and the commands
// scenarios run must be one set, and every documented GASCOIGNE_* variable must be exercised.
// An exemption names why a command or variable has no scenario; a stale one fails the test, so
// the list can only shrink.

// notCovered maps a command to why no scenario runs it yet.
var notCovered = map[string]string{}

// notExercised maps a documented environment variable to why no scenario sets it yet.
var notExercised = map[string]string{
	"GASCOIGNE_CAT_MODE":      "no scenario yet",
	"GASCOIGNE_SERVER_IDLE":   "needs the background server",
	"GASCOIGNE_SERVER_ADDR":   "needs the background server",
	"GASCOIGNE_MCP_LAZY":      "needs mcp-proxy",
	"GASCOIGNE_CONTEXT_NUDGE": "needs an agent hook",
	"GASCOIGNE_RELEASE_URL":   "needs a release page to read",
	"GASCOIGNE_GUIDE":         "needs install",
	"GASCOIGNE_MCP_MODE":      "needs install",
	"GASCOIGNE_PROXY":         "the sandbox sets it to off; needs an agent install",
	"GASCOIGNE_MCP_REVIEW":    "the sandbox sets it to off; needs an agent session start",
	"GASCOIGNE_RELEASE_CHECK": "the sandbox sets it to off; needs a release page to read",
	"GASCOIGNE_AUTOSYNC":      "the sandbox sets it to off; needs an agent hook after an upgrade",
}

func TestEveryCommandHasScenario(t *testing.T) {
	used := commandsUsed()
	for _, command := range overviewCommands(t) {
		reason, exempt := notCovered[command]
		switch {
		case used[command] && exempt:
			t.Errorf("%s has a scenario now: drop it from notCovered (%q)", command, reason)
		case !used[command] && !exempt:
			t.Errorf("%s has no scenario: add one, or exempt it in notCovered with a reason", command)
		}
	}
}

// A bug waiting to happen: an exemption for a command the overview no longer lists would never
// be checked again, so it must go.
func TestNoStaleCommandExemptions(t *testing.T) {
	listed := overviewCommands(t)
	for command := range notCovered {
		if !slices.Contains(listed, command) {
			t.Errorf("notCovered has %s, which `gascoigne help` no longer lists", command)
		}
	}
}

func TestDocsListEveryCommand(t *testing.T) {
	documented := documentedCommands(t)
	for _, command := range overviewCommands(t) {
		if !slices.Contains(documented, command) {
			t.Errorf("%s is in `gascoigne help` but not in the command table of docs/cli.md", command)
		}
	}
}

func TestEveryDocumentedEnvVarIsExercised(t *testing.T) {
	exercised := variablesUsed()
	for _, variable := range documentedVariables(t) {
		reason, exempt := notExercised[variable]
		switch {
		case exercised[variable] && exempt:
			t.Errorf("%s is exercised now: drop it from notExercised (%q)", variable, reason)
		case !exercised[variable] && !exempt:
			t.Errorf("%s is documented but no scenario sets it: add one, or exempt it in notExercised", variable)
		}
	}
}

// commandsUsed is the first word after `gascoigne` of every step of every scenario.
func commandsUsed() map[string]bool {
	used := map[string]bool{}
	for _, entry := range registry {
		for _, scenario := range entry.scenarios {
			for _, step := range scenario.Steps {
				words := splitQuoted(step.Run)
				if len(words) > 1 && words[0] == "gascoigne" {
					used[words[1]] = true
				}
			}
		}
	}
	return used
}

// variableName and the row patterns compile in a function: package-level regexps use lazyre.
func variableName() *regexp.Regexp { return regexp.MustCompile(`GASCOIGNE_[A-Z_]+`) }

// variablesUsed is every GASCOIGNE_* name a scenario or step sets. The sandbox's own defaults
// do not count: they switch things off, they do not test them.
func variablesUsed() map[string]bool {
	used := map[string]bool{}
	for _, entry := range registry {
		for _, scenario := range entry.scenarios {
			for _, text := range scenario.Env {
				used[variableName().FindString(text)] = true
			}
			for _, step := range scenario.Steps {
				for _, text := range step.Env {
					used[variableName().FindString(text)] = true
				}
			}
		}
	}
	return used
}

// overviewCommands reads the command names from the sections of `gascoigne help`.
func overviewCommands(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	help := exec.Command(toolPath["gascoigne"], "help")
	help.Env = []string{"HOME=" + home, "XDG_CONFIG_HOME=" + home, "XDG_DATA_HOME=" + home,
		"XDG_CACHE_HOME=" + home, "XDG_STATE_HOME=" + home, "GASCOIGNE_SERVER=off", "NO_COLOR=1"}
	out, err := help.Output()
	if err != nil {
		t.Fatalf("gascoigne help: %v", err)
	}
	var commands []string
	inSections := false
	for line := range strings.SplitSeq(string(out), "\n") {
		switch {
		case line == "AGENTS":
			inSections = true
		case line == "GET STARTED":
			return commands
		case inSections && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   "):
			commands = append(commands, strings.Fields(line)[0])
		}
	}
	t.Fatal("gascoigne help: no GET STARTED section; did the overview change shape?")
	return nil
}

// documentedCommands is the first word of each row of the command table in docs/cli.md.
func documentedCommands(t *testing.T) []string {
	t.Helper()
	commandRow := regexp.MustCompile("^\\| `([a-z][a-z-]*)")
	var commands []string
	for line := range strings.SplitSeq(readDoc(t), "\n") {
		if match := commandRow.FindStringSubmatch(line); match != nil {
			commands = append(commands, match[1])
		}
	}
	return commands
}

// documentedVariables is every GASCOIGNE_* name opening a row of the environment table.
func documentedVariables(t *testing.T) []string {
	t.Helper()
	variableRow := regexp.MustCompile("^\\| `(GASCOIGNE_[A-Z_]+)")
	var variables []string
	for line := range strings.SplitSeq(readDoc(t), "\n") {
		if variableRow.MatchString(line) {
			variables = append(variables, variableName().FindAllString(strings.SplitN(line, "|", 3)[1], -1)...)
		}
	}
	return variables
}

func readDoc(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repoRoot, "docs", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
