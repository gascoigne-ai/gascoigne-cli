package stdoutexporter

import (
	"context"
	"os"

	"github.com/gascoigne-ai/gascoigne-cli/component"
)

const typ = "stdout"

func NewFactory() component.Factory {
	return component.NewFactory(component.KindExporter, typ,
		func() *Config { return &Config{Hint: true} },
		func(_ context.Context, _ component.Settings, cfg *Config) (component.Component, error) {
			return &Exporter{cfg: cfg, w: os.Stdout}, nil
		})
}
