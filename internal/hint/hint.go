// Package hint is the line that ends optimized output and marks it as
// shortened. It costs the agent tokens on every shortened result, so it is as
// small as it can be: only the id of the original. Where the original is read
// (gascoigne raw <id>, the retrieve tool) is told once, in the guide agents get
// at session start. Token counts are left out: the agent cannot use them; the
// ledger and the dashboard keep them.
package hint

import "github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"

// Of is the hint for the original saved under id.
func Of(id string) string { return "[raw:" + id + "]" }

// Re finds output gascoigne already optimized, so it is not optimized twice.
// It also matches the longer hint older versions wrote: transcripts and
// sessions started before the change still hold it.
var Re = lazyre.New(`\[raw:\w+\]|\[(?:gascoigne: )?optimized \d+→\d+ tokens · full: `)
