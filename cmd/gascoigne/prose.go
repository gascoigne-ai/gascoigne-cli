package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gascoigne-ai/gascoigne-cli/prose"
)

// cmdProse sets up and serves the prose model. serve is what the doc
// optimizer starts as its child, so neither subcommand goes through the
// background server.
func cmdProse(ctx context.Context, args []string) error {
	sub := "install"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	fs := newFlags("prose")
	dir := fs.String("model", "", "model `directory` (default: user cache dir, gascoigne/prose-model)")
	lib := fs.String("lib", os.Getenv("ONNXRUNTIME_LIB"), "ONNX Runtime shared library (default: the one install put in the model directory, else the system's)")
	device := fs.String("device", "auto", "serve: auto, cuda, coreml or cpu")
	variant := fs.String("runtime", "auto", "install: ONNX Runtime build: auto (by nvidia-smi), cpu, cuda12 or cuda13")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		d, err := prose.ModelDir()
		if err != nil {
			return err
		}
		*dir = d
	}
	switch sub {
	case "install":
		return prose.Install(ctx, *dir, *variant)
	case "serve":
		return prose.Serve(os.Stdin, os.Stdout, *dir, *lib, *device)
	}
	return fmt.Errorf("unknown prose command %q (install, serve)", sub)
}
