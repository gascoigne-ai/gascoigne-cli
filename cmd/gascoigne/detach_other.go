//go:build !unix

package main

import "os/exec"

func detach(*exec.Cmd) {}
