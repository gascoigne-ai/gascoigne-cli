// Package registry lists every built-in component. Adding a component is one line here.
package registry

import (
	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/exporter/stdoutexporter"
	"github.com/gascoigne-ai/gascoigne-cli/extension/ledger"
	"github.com/gascoigne-ai/gascoigne-cli/filter/bypassfilter"
	"github.com/gascoigne-ai/gascoigne-cli/filter/detectfilter"
	"github.com/gascoigne-ai/gascoigne-cli/filter/historyfilter"
	"github.com/gascoigne-ai/gascoigne-cli/filter/minsizefilter"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/ariaoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/buildoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/catoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/cloudoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/dataoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/dboptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/denseoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/docoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/embedjsonoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/gitoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/grepoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/htmloptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/httpoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/infraoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/jsonoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/k8soptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/logoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/lsoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/passthroughoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/pathsoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/pkgoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/proseoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/rerunoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/tableoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/taskoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/testoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/traceoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/truncateoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/execreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/hookreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/httpproxyreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpproxyreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/mcpreceiver"
	"github.com/gascoigne-ai/gascoigne-cli/receiver/stdinreceiver"
)

func Factories() []component.Factory {
	return []component.Factory{
		// receivers
		stdinreceiver.NewFactory(),
		execreceiver.NewFactory(),
		hookreceiver.NewFactory(),
		mcpreceiver.NewFactory(),
		mcpproxyreceiver.NewFactory(),
		httpproxyreceiver.NewFactory(),
		// filters
		bypassfilter.NewFactory(),
		minsizefilter.NewFactory(),
		detectfilter.NewFactory(),
		historyfilter.NewFactory(),
		// optimizers
		passthroughoptimizer.NewFactory(),
		denseoptimizer.NewFactory(),
		rerunoptimizer.NewFactory(),
		traceoptimizer.NewFactory(),
		embedjsonoptimizer.NewFactory(),
		pathsoptimizer.NewFactory(),
		truncateoptimizer.NewFactory(),
		testoptimizer.NewFactory(),
		buildoptimizer.NewFactory(),
		gitoptimizer.NewFactory(),
		grepoptimizer.NewFactory(),
		lsoptimizer.NewFactory(),
		jsonoptimizer.NewFactory(),
		dataoptimizer.NewFactory(),
		logoptimizer.NewFactory(),
		catoptimizer.NewFactory(),
		pkgoptimizer.NewFactory(),
		tableoptimizer.NewFactory(),
		dboptimizer.NewFactory(),
		k8soptimizer.NewFactory(),
		cloudoptimizer.NewFactory(),
		infraoptimizer.NewFactory(),
		taskoptimizer.NewFactory(),
		httpoptimizer.NewFactory(),
		htmloptimizer.NewFactory(),
		ariaoptimizer.NewFactory(),
		docoptimizer.NewFactory(),
		proseoptimizer.NewFactory(),
		// exporters
		stdoutexporter.NewFactory(),
		// extensions
		ledger.NewFactory(),
	}
}
