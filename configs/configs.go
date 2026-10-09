// Package configs embeds the default config so the binary works without files.
package configs

import _ "embed"

//go:embed default.yaml
var Default []byte
