package table

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var reportHeader = lazyre.New(`^\s*case\s{2,}type\s{2,}.*\bout tok\s.*\scheck\s*$`)

// FoldReport shortens a report of several tables with a `check` column
// (gascoigne eval): rows that passed fold to one count per table; failing
// and skipped rows, totals and all text between the tables stay.
func FoldReport(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); i++ {
		out = append(out, lines[i])
		if !reportHeader.MatchString(lines[i]) {
			continue
		}
		ok := 0
		for i++; i < len(lines) && strings.TrimSpace(lines[i]) != ""; i++ {
			if f := strings.Fields(lines[i]); f[len(f)-1] == "ok" {
				ok++
				continue
			}
			out = append(out, lines[i])
		}
		if ok > 0 {
			out = append(out, fmt.Sprintf(" … %d ok", ok))
		}
		i-- // the blank line ends the table; the loop prints it
	}
	return out
}
