//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the server in its own session: closing the agent's terminal
// or killing its process group leaves the server running.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

// terminate asks a process to exit.
func terminate(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) }
