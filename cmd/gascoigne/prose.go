package main

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/prose"
)

// proseStatus says whether the prose model pass can run, and what to do when
// it cannot. State is off, not-installed or ready.
type proseStatus struct {
	State  string
	Detail string
}

func loadProseStatus() proseStatus {
	if !experimentalOn("prose") {
		return proseStatus{"off", "add prose to experimental in the config"}
	}
	dir, _ := prose.ModelDir()
	name := configProseModel()
	if model, rt := prose.Installed(dir, name); !model || !rt {
		return proseStatus{"not-installed", "run: gascoigne prose install -name " + name}
	}
	return proseStatus{"ready", name + " model files in " + dir}
}

// cmdProse sets up the prose model and reports whether it can run. The model
// itself is loaded by the prose optimizer, in the process that optimizes.
func cmdProse(ctx context.Context, args []string) error {
	sub := "install"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		sub, args = args[0], args[1:]
	}
	fs := newFlags("prose")
	dir := fs.String("model", "", "model `directory` (default: user cache dir, gascoigne/prose-model)")
	name := fs.String("name", "", "install: model, "+strings.Join(prose.Names(), " or ")+" (default: optimizers.prose.model of the config)")
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
		return prose.Install(ctx, *dir, *variant, cmp.Or(*name, configProseModel()))
	}
	return fmt.Errorf("unknown prose command %q (install, status)", sub)
}
