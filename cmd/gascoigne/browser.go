package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// openBrowser opens url in the default browser. Never fails the command: the
// address is already printed. Without a display (ssh) it does nothing.
func openBrowser(url string) {
	if !hasDisplay(runtime.GOOS) {
		return
	}
	argv := browserCmd(runtime.GOOS, url)
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: cannot open browser:", err)
		return
	}
	cmd.Process.Release()
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
