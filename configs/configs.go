// Package configs embeds the default config so the binary works without files.
package configs

import _ "embed"

//go:embed default.yaml
var Default []byte

// User is the documented template written to the user's config dir on first
// run: every setting, commented out, so the defaults apply until changed.
//
//go:embed user.yaml
var User []byte
