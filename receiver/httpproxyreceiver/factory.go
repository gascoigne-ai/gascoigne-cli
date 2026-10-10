package httpproxyreceiver

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
)

const typ = "httpproxy"

// DefaultListen is where agents reach the proxy unless configured otherwise.
const DefaultListen = "127.0.0.1:7879"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config {
			return &Config{
				Listen: DefaultListen, Anthropic: "https://api.anthropic.com", OpenAI: "https://api.openai.com",
				Skip:         []string{"Edit", "MultiEdit", "Write", "NotebookEdit", "TodoWrite", "ExitPlanMode", "AskUserQuestion", "apply_patch"},
				CacheEntries: 50000,
			}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return newReceiver(cfg, set.Pipeline, set.Logger), nil
		})
}
