// Package registry lists every built-in component. Adding a component is one line here.
package registry

import (
	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/exporter/stdoutexporter"
	"github.com/lukaszb/gascoigne/internal/extension/ledger"
	"github.com/lukaszb/gascoigne/internal/filter/bypassfilter"
	"github.com/lukaszb/gascoigne/internal/filter/historyfilter"
	"github.com/lukaszb/gascoigne/internal/filter/minsizefilter"
	"github.com/lukaszb/gascoigne/internal/optimizer/passthroughoptimizer"
	"github.com/lukaszb/gascoigne/internal/receiver/stdinreceiver"
)

func Factories() []component.Factory {
	return []component.Factory{
		// receivers
		stdinreceiver.NewFactory(),
		// filters
		bypassfilter.NewFactory(),
		minsizefilter.NewFactory(),
		historyfilter.NewFactory(),
		// optimizers
		passthroughoptimizer.NewFactory(),
		// exporters
		stdoutexporter.NewFactory(),
		// extensions
		ledger.NewFactory(),
	}
}
