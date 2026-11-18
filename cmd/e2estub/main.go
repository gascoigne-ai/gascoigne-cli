// Command e2estub is the fake external program of the e2e suite. Its persona is the name it is
// started under, so one binary symlinked as several names stands in for several programs.
//
//	emit [-exit n] [-stderr file] [file...]  print files to stdout, one file to stderr, exit n
//	claude codex gemini opencode pi          an agent: write argv and GASCOIGNE_* env as JSON to
//	                                         $E2E_RECORD, exit with $E2E_EXIT
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	name := filepath.Base(os.Args[0])
	switch name {
	case "emit":
		os.Exit(emit(os.Args[1:]))
	case "claude", "codex", "gemini", "opencode", "pi":
		os.Exit(agent(name, os.Args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "e2estub: no persona %q\n", name)
		os.Exit(126)
	}
}

// emit prints fixtures the way a tool prints its output, so a scenario controls
// exactly what the optimizer sees and what exit code the child returns.
func emit(args []string) int {
	flags := flag.NewFlagSet("emit", flag.ContinueOnError)
	code := flags.Int("exit", 0, "exit code")
	stderrFile := flags.String("stderr", "", "file printed to stderr")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *stderrFile != "" && !copyFile(os.Stderr, *stderrFile) {
		return 2
	}
	for _, path := range flags.Args() {
		if !copyFile(os.Stdout, path) {
			return 2
		}
	}
	return *code
}

func copyFile(dst *os.File, path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2estub:", err)
		return false
	}
	dst.Write(data)
	return true
}

// agent stands in for a coding agent started by `wrap`: it records how it was started, so a
// scenario can check the arguments and the GASCOIGNE_* variables the agent would have seen.
func agent(name string, args []string) int {
	record := map[string]any{"agent": name, "args": args, "env": gascoigneEnv()}
	data, err := json.Marshal(record)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2estub:", err)
		return 2
	}
	if path := os.Getenv("E2E_RECORD"); path != "" {
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "e2estub:", err)
			return 2
		}
	}
	code, _ := strconv.Atoi(os.Getenv("E2E_EXIT"))
	return code
}

func gascoigneEnv() map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		if key, value, ok := strings.Cut(entry, "="); ok && strings.HasPrefix(key, "GASCOIGNE_") {
			env[key] = value
		}
	}
	return env
}
