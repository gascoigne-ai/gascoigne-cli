package main

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"
)

// openBrowser opens url in the default browser. Never fails the command: the
// URL is printed anyway, a missing browser only costs a click.
func openBrowser(url string) { tryOpenBrowser(url) }

// tryOpenBrowser reports whether a browser was started. An agent starts mcp-proxy
// and the background server with its own environment, which often lacks the
// desktop session (no DISPLAY, no bus); sessionEnv finds it so the page still
// opens, from a process gascoigne owns, as dashboard does.
func tryOpenBrowser(url string) bool {
	env := os.Environ()
	if !hasDisplay(runtime.GOOS) {
		extra := sessionEnv(runtime.GOOS, runtimeDir())
		if len(extra) == 0 {
			return false
		}
		env = append(env, extra...)
	}
	argv := browserCmd(runtime.GOOS, url)
	if runtime.GOOS == "linux" {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Env = env
		return start(cmd) // xdg-open may stay until the browser exits
	}
	// open and rundll32 return at once, and with an error when there is no
	// session to open the page in (a service, an ssh login): wait for that answer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := exec.CommandContext(ctx, argv[0], argv[1:]...).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: cannot open browser:", err)
		return false
	}
	return true
}

func start(cmd *exec.Cmd) bool {
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: cannot open browser:", err)
		return false
	}
	cmd.Process.Release()
	return true
}

func runtimeDir() string {
	return cmp.Or(os.Getenv("XDG_RUNTIME_DIR"), fmt.Sprintf("/run/user/%d", os.Getuid()))
}

// sessionEnv is the desktop session of the user on Linux, found from its
// sockets when the environment names none: a Wayland socket or the X server in
// runtime, plus the session bus. Nil when there is no session to open a page in.
func sessionEnv(goos, dir string) []string {
	if goos != "linux" {
		return nil
	}
	var env []string
	if socks, _ := filepath.Glob(filepath.Join(dir, "wayland-*")); len(socks) > 0 {
		slices.Sort(socks)
		for _, s := range socks {
			if !strings.HasSuffix(s, ".lock") {
				env = append(env, "WAYLAND_DISPLAY="+filepath.Base(s))
				break
			}
		}
	}
	if len(env) == 0 {
		if _, err := os.Stat("/tmp/.X11-unix/X0"); err == nil {
			env = append(env, "DISPLAY=:0")
		}
	}
	if len(env) == 0 {
		return nil
	}
	env = append(env, "XDG_RUNTIME_DIR="+dir)
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		if _, err := os.Stat(filepath.Join(dir, "bus")); err == nil {
			env = append(env, "DBUS_SESSION_BUS_ADDRESS=unix:path="+filepath.Join(dir, "bus"))
		}
	}
	return env
}

// hasDisplay reports whether a browser can open here: macOS and Windows
// always, other systems with a display server (not over ssh).
func hasDisplay(goos string) bool {
	return goos == "darwin" || goos == "windows" || os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

// browserCmd is the command that opens url on goos.
func browserCmd(goos, url string) []string {
	switch goos {
	case "darwin":
		return []string{"open", url}
	case "windows":
		return []string{"rundll32", "url.dll,FileProtocolHandler", url}
	}
	return []string{"xdg-open", url}
}
