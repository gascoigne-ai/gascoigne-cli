package httpproxyreceiver

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "httpproxy"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindReceiver, typ,
		func() *Config {
			return &Config{
				Anthropic: "https://api.anthropic.com", OpenAI: "https://api.openai.com",
				Gemini:       "https://generativelanguage.googleapis.com",
				Bedrock:      "https://bedrock-runtime.{region}.amazonaws.com",
				Skip:         []string{"Edit", "MultiEdit", "Write", "NotebookEdit", "TodoWrite", "ExitPlanMode", "AskUserQuestion", "apply_patch"},
				CacheEntries: 50000, Ledger: "ledger", FoldMinLines: 3, FoldMinChars: 80, CompactTools: true, MaxImagePixels: 0, SkillCodeLines: 30,
			}
		},
		func(_ context.Context, set component.Settings, cfg *Config) (component.Component, error) {
			return New(cfg, set.Pipeline, set.Logger), nil
		})
}
