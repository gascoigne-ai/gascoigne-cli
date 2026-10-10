//go:build !unix

package main

import (
	"errors"
	"os/exec"
)

func detach(*exec.Cmd) {}

// terminate is not supported here: an outdated proxy keeps running.
func terminate(int) error { return errors.ErrUnsupported }
