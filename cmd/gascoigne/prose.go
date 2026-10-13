package main

import (
	"context"
	"fmt"

	"github.com/gascoigne-ai/gascoigne-cli/prose"
)

// proseStatus says whether the prose model pass can run, and what to do when
// it cannot. State is off, not-installed or ready.
type proseStatus struct {
	State   string `json:"state"`
	Detail  string `json:"detail"`
	Enabled bool   `json:"enabled"` // the experimental prose setting is on
	Dir     string `json:"dir"`
}

func loadProseStatus() proseStatus {
	st := proseStatus{State: "off", Detail: "add prose to experimental in the config"}
	if !experimentalOn("prose") {
		return st
	}
	st.Enabled = true
	st.Dir, _ = prose.ModelDir()
	if model, rt := prose.Installed(st.Dir); !model || !rt {
		st.State, st.Detail = "not-installed", "run: gascoigne prose install"
		return st
	}
	st.State, st.Detail = "ready", "model files in "+st.Dir
	return st
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
	}
	return fmt.Errorf("unknown prose command %q (install, status)", sub)
}
