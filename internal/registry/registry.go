// Package registry lists every built-in component. Adding a component is one line here.
package registry

import (
	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/exporter/stdoutexporter"
	"github.com/lukaszb/gascoigne/extension/ledger"
	"github.com/lukaszb/gascoigne/filter/bypassfilter"
	"github.com/lukaszb/gascoigne/filter/detectfilter"
	"github.com/lukaszb/gascoigne/filter/historyfilter"
	"github.com/lukaszb/gascoigne/filter/minsizefilter"
	"github.com/lukaszb/gascoigne/optimizer/build/buildoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/passthroughoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/data/jsonoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/data/logoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/fs/catoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/fs/lsoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/js/bunoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/js/npmoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/js/nxoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/search/grepoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/test/testoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/vcs/gitoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/web/htmloptimizer"
	"github.com/lukaszb/gascoigne/receiver/execreceiver"
	"github.com/lukaszb/gascoigne/receiver/mcpproxyreceiver"
	"github.com/lukaszb/gascoigne/receiver/mcpreceiver"
	"github.com/lukaszb/gascoigne/receiver/stdinreceiver"
)

func Factories() []component.Factory {
	return []component.Factory{
		// receivers
		stdinreceiver.NewFactory(),
		execreceiver.NewFactory(),
		mcpreceiver.NewFactory(),
		mcpproxyreceiver.NewFactory(),
		// filters
		bypassfilter.NewFactory(),
		minsizefilter.NewFactory(),
		detectfilter.NewFactory(),
		historyfilter.NewFactory(),
		// optimizers
		passthroughoptimizer.NewFactory(),
		testoptimizer.NewFactory(),
		buildoptimizer.NewFactory(),
		gitoptimizer.NewFactory(),
		grepoptimizer.NewFactory(),
		lsoptimizer.NewFactory(),
		jsonoptimizer.NewFactory(),
		logoptimizer.NewFactory(),
		catoptimizer.NewFactory(),
		npmoptimizer.NewFactory(),
		bunoptimizer.NewFactory(),
		nxoptimizer.NewFactory(),
		htmloptimizer.NewFactory(),
		// exporters
		stdoutexporter.NewFactory(),
		// extensions
		ledger.NewFactory(),
	}
}
