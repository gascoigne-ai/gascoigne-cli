package httpproxyreceiver

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "httpproxy"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config {
			return &Config{
				Anthropic: "https://api.anthropic.com", OpenAI: "https://api.openai.com",
				Skip:         []string{"Edit", "MultiEdit", "Write", "NotebookEdit", "TodoWrite", "ExitPlanMode", "AskUserQuestion", "apply_patch"},
				CacheEntries: 50000,
			}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return New(cfg, set.Pipeline, set.Logger), nil
		})
}
