#!/usr/bin/env bash
# Boundary guard. Public contracts (component, payload, tokens) must not import internal/.
# Components may reuse internal libs (compress, detect, ...) but never the engine
# (internal/{config,router,service,registry}), which is what wires them.
set -euo pipefail
cd "$(dirname "$0")/../.."

public=(./component/... ./payload/... ./tokens/...)
bad=$(go list -f '{{.ImportPath}}: {{join .Imports " "}} {{join .TestImports " "}}' "${public[@]}" |
	grep -E '/internal(/|$| )' || true)
if [[ -n $bad ]]; then
	echo "public packages must not import internal/:" >&2
	echo "$bad" >&2
	exit 1
fi

components=(./receiver/... ./filter/... ./optimizer/... ./exporter/... ./extension/...)
bad=$(go list -f '{{.ImportPath}}: {{join .Imports " "}} {{join .TestImports " "}}' "${components[@]}" |
	grep -E 'internal/(config|router|service|registry)([/ ]|$)' || true)
if [[ -n $bad ]]; then
	echo "components must not import the engine (internal/{config,router,service,registry}):" >&2
	echo "$bad" >&2
	exit 1
fi
