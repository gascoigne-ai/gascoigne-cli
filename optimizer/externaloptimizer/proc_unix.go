//go:build unix

package externaloptimizer

import (
	"os/exec"
	"syscall"
)

// isolate gives the child its own process group and makes cancel kill the whole group.
func isolate(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
