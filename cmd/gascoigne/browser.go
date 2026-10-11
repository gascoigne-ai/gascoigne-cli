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
	goos := runtime.GOOS
	if goos != "darwin" && goos != "windows" && os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return
	}
	argv := browserCmd(goos, url)
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "gascoigne: cannot open browser:", err)
		return
	}
	cmd.Process.Release()
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
