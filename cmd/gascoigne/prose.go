package main

import (
	"context"
	"fmt"
	"os"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/prose"
)

// proseStatus says whether the prose model pass can run, and what to do when
// it cannot. State is off, needs-experimental, not-installed or ready.
type proseStatus struct {
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Enabled bool   `json:"enabled"` // optimizers.prose.enabled
	Dir     string `json:"dir"`
}

func loadProseStatus() proseStatus {
	st := proseStatus{State: "off", Detail: "optimizers.prose.enabled is false"}
	data, err := configData("")
	if err != nil {
		st.Detail = err.Error()
		return st
	}
	var c struct {
		Optimizers struct {
			Prose struct {
				Enabled bool `yaml:"enabled"`
			} `yaml:"prose"`
		} `yaml:"optimizers"`
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		st.Detail = err.Error()
		return st
	}
	st.Enabled = c.Optimizers.Prose.Enabled
	if !st.Enabled {
		return st
	}
	st.Dir, _ = prose.ModelDir()
	model, rt := prose.Installed(st.Dir)
	switch {
	case !experimentalOn("prose"):
		st.State, st.Detail = "needs-experimental", "add prose to experimental in the config"
	case !model || !rt:
		st.State, st.Detail = "not-installed", "run: gascoigne prose install"
	default:
		st.State, st.Detail = "ready", "model files in "+st.Dir
	}
	return st
}

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
	case "status":
		st := loadProseStatus()
		fmt.Printf("prose: %s (%s)\n", st.State, st.Detail)
		return nil
	case "install":
		return prose.Install(ctx, *dir, *variant)
	case "serve":
		return prose.Serve(os.Stdin, os.Stdout, *dir, *lib, *device)
	}
	return fmt.Errorf("unknown prose command %q (install, serve, status)", sub)
}
