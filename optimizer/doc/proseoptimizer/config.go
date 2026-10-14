package proseoptimizer

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/prose"
)

type Config struct {
	// Enabled is set by the experimental prose setting (see the prose command);
	// setting it false there vetoes it.
	Enabled bool `yaml:"enabled"`
	// Device is auto (CUDA or CoreML, else CPU), cuda, coreml or cpu.
	Device string `yaml:"device"`
	// Model is the prose model, by Hugging Face name: kompress-v2-base (a general
	// compressor) or ctxprune-small (also trained on tool output, logs and code;
	// run prose install -name ctxprune-small for it).
	Model string `yaml:"model"`
	// Threshold is the keep score from which a word stays; higher drops more.
	Threshold float64 `yaml:"threshold"`
	// Warm loads the model when the optimizer starts, so the server's first
	// request does not wait for it.
	Warm bool `yaml:"warm"`
	// Idle unloads the model (freeing GPU memory) after this long without a
	// paragraph; the next one loads it again, which takes seconds. 0 keeps it.
	Idle time.Duration `yaml:"idle"`
	// MinWords skips shorter paragraphs: dropping a word or two saves nothing.
	MinWords int `yaml:"min_words"`
}

func (c *Config) Validate() error {
	if c.Enabled && !slices.Contains([]string{"auto", "cuda", "coreml", "cpu"}, c.Device) {
		return errors.New("device must be auto, cuda, coreml or cpu")
	}
	if c.Enabled && !slices.Contains(prose.Names(), c.Model) {
		return fmt.Errorf("model must be one of %v", prose.Names())
	}
	if c.Enabled && (c.Threshold <= 0 || c.Threshold >= 1 || c.MinWords < 1) {
		return errors.New("need 0 < threshold < 1 and min_words >= 1")
	}
	if c.Idle < 0 {
		return errors.New("idle must not be negative")
	}
	return nil
}
