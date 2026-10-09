// Package registry lists every built-in component. Adding a component is one line here.
package registry

import (
	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/exporter/stdoutexporter"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/filter/bypassfilter"
	"github.com/lukaszb/gascoigne/filter/historyfilter"
	"github.com/lukaszb/gascoigne/filter/minsizefilter"
	"github.com/lukaszb/gascoigne/optimizer/passthroughoptimizer"
	"github.com/lukaszb/gascoigne/receiver/stdinreceiver"
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
