package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
)

// cmdUninstall removes everything gascoigne leaves behind: its entries in
// user and project agent configs (as unwrap), the background server, its
// config, data, cache and state dirs, and the binary itself.
func cmdUninstall(ctx context.Context, args []string) error {
	fset := newFlags("uninstall")
	yes := fset.Bool("y", false, "do not ask for confirmation")
	if err := fset.Parse(args); err != nil {
		return err
	}
	addr := serverAddr()
	info, err := health(addr)
	running := err == nil
	files := leftovers(info.Ledger)
	exe, exeErr := os.Executable()

	fmt.Println("gascoigne uninstall removes:")
	fmt.Println("  gascoigne entries in user and this project's agent configs (backups kept)")
	if running {
		fmt.Printf("  the server on %s (pid %d)\n", addr, info.PID)
	}
	for _, f := range files {
		fmt.Println("  " + display(f))
	}
	if exeErr == nil {
		fmt.Println("  " + display(exe))
	}
	if !*yes && !confirm(os.Stdin, os.Stdout) {
		fmt.Println("gascoigne: nothing removed")
		return nil
	}

	errs := []error{cmdUnwrap(ctx, nil)} // before the state dir: it reads denies.json
	if running {
		errs = append(errs, stopServer(addr))
	}
	for _, f := range files {
		errs = append(errs, os.RemoveAll(f))
	}
	if exeErr == nil {
		errs = append(errs, os.Remove(exe))
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("uninstall: %w", err)
	}
	fmt.Println("gascoigne: uninstalled; other projects' agent configs: run gascoigne unwrap there first")
	return nil
}

// leftovers are the existing paths gascoigne writes outside agent configs:
// its config, data, cache and state dirs, and the ledger with its sqlite
// sidecars when it lives elsewhere ($GASCOIGNE_LEDGER, the server's).
// A custom ledger path loses only its files, never its dir.
func leftovers(ledgers ...string) []string {
	home, _ := os.UserHomeDir()
	var dirs []string
	if dir, err := os.UserConfigDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "gascoigne"))
	}
	dirs = append(dirs,
		filepath.Join(xdgDir("XDG_DATA_HOME", filepath.Join(home, ".local", "share")), "gascoigne"),
		filepath.Join(xdgDir("XDG_STATE_HOME", filepath.Join(home, ".local", "state")), "gascoigne"))
	if dir, err := os.UserCacheDir(); err == nil {
		dirs = append(dirs, filepath.Join(dir, "gascoigne"))
	}
	if p, err := (&ledger.Config{}).File(); err == nil {
		ledgers = append(ledgers, p)
	}
	var found []string
	for _, p := range dirs {
		if exists(p) && !slices.Contains(found, p) {
			found = append(found, p)
		}
	}
	for _, l := range ledgers {
		if l == "" || slices.ContainsFunc(dirs, func(d string) bool { return strings.HasPrefix(l, d+string(filepath.Separator)) }) {
			continue
		}
		for _, p := range []string{l, l + "-wal", l + "-shm"} {
			if exists(p) && !slices.Contains(found, p) {
				found = append(found, p)
			}
		}
	}
	return found
}

// xdgDir is $env, or fallback when it is unset.
func xdgDir(env, fallback string) string {
	if dir := os.Getenv(env); dir != "" {
		return dir
	}
	return fallback
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// confirm asks on w and reads the answer from r; only y or yes agrees.
func confirm(r io.Reader, w io.Writer) bool {
	fmt.Fprint(w, "Proceed? [y/N] ")
	line, _ := bufio.NewReader(r).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
