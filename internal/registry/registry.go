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
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/build/buildoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/cloud/cloudoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/denseoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/embedjsonoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/passthroughoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/pathsoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/rerunoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/traceoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/core/truncateoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/data/dataoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/data/jsonoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/db/dboptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/doc/docoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/doc/proseoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/fs/catoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/fs/lsoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/infra/infraoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/k8s/k8soptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/log/logoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/pkg/pkgoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/search/grepoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/table/tableoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/task/taskoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/test/testoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/vcs/gitoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/web/ariaoptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/web/htmloptimizer"
	"github.com/gascoigne-ai/gascoigne-cli/optimizer/web/httpoptimizer"
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
