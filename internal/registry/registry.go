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
	"github.com/lukaszb/gascoigne/optimizer/cloud/cloudoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/denseoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/embedjsonoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/passthroughoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/pathsoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/rerunoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/traceoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/core/truncateoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/data/dataoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/data/jsonoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/db/dboptimizer"
	"github.com/lukaszb/gascoigne/optimizer/doc/docoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/fs/catoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/fs/lsoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/infra/infraoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/k8s/k8soptimizer"
	"github.com/lukaszb/gascoigne/optimizer/log/logoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/pkg/pkgoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/search/grepoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/table/tableoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/task/taskoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/test/testoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/vcs/gitoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/web/ariaoptimizer"
	"github.com/lukaszb/gascoigne/optimizer/web/htmloptimizer"
	"github.com/lukaszb/gascoigne/optimizer/web/httpoptimizer"
	"github.com/lukaszb/gascoigne/receiver/execreceiver"
	"github.com/lukaszb/gascoigne/receiver/hookreceiver"
	"github.com/lukaszb/gascoigne/receiver/httpproxyreceiver"
	"github.com/lukaszb/gascoigne/receiver/mcpproxyreceiver"
	"github.com/lukaszb/gascoigne/receiver/mcpreceiver"
	"github.com/lukaszb/gascoigne/receiver/stdinreceiver"
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
		// exporters
		stdoutexporter.NewFactory(),
		// extensions
		ledger.NewFactory(),
	}
}
