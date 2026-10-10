#!/usr/bin/env bash
# Module boundary guard: public packages (contracts + components) must not import
# internal/, so each component dir can later become its own Go module.
set -euo pipefail
cd "$(dirname "$0")/../.."

public=(./component/... ./payload/... ./tokens/... ./detect/... ./compress/... ./mcptool/... ./cost/... ./lazyre/... ./receiver/... ./filter/... ./optimizer/... ./exporter/... ./extension/...)
bad=$(go list -f '{{.ImportPath}}: {{join .Imports " "}} {{join .TestImports " "}}' "${public[@]}" |
	grep -E '/internal(/|$| )' || true)
if [[ -n $bad ]]; then
	echo "public packages must not import internal/:" >&2
	echo "$bad" >&2
	exit 1
fi
