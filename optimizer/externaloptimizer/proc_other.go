//go:build !unix

package externaloptimizer

import "os/exec"

// isolate is a no-op here: exec.CommandContext's default cancel kills the child only.
func isolate(*exec.Cmd) {}
