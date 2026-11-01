// Package pkglist compresses package inventories (pip list|freeze, apt list,
// pacman -Q, brew list --versions, gem list, go list -m all) to one
// "name version" line per package. Padding, header underlines and per-tool
// decoration go; consecutive packages with the same version share a line
// (`5.87-2: bluez bluez-libs bluez-utils`).
package pkglist

import (
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
)

// noise lines carry no package.
var noise = pcomb.Rules{
	{
		Name:  "blank and rulers",
		Match: pcomb.Re(`^\s*$|^-+(\s+-+)*\s*$`),
		Yes:   []string{"", "  ", "------- -------", "-----"},
		No:    []string{"Flask 3.0.3", "-e git+https://x"},
	},
	{
		Name:  "list headers",
		Match: pcomb.Any(pcomb.Prefix("Listing...", "*** LOCAL GEMS ***"), pcomb.Re(`^Package\s+Version\s*$`)),
		Yes:   []string{"Listing... Done", "*** LOCAL GEMS ***", "Package    Version", "Package Version"},
		No:    []string{"Package Version Latest Type", "package 1.0"},
	},
}

// form turns a line into a package: match group 1 is the name, key what is
// left of the line (version and decoration).
type form struct {
	re  *lazyre.Regexp
	key func(m []string) string
}

var forms = []form{
	// apt: `curl/noble-updates,now 8.5.0-2 amd64 [installed,automatic]`
	{lazyre.New(`^([^\s/]+)/\S+ (\S+) (\S+)(?: \[(.*)\])?$`), func(m []string) string {
		flags := strings.TrimPrefix(strings.TrimPrefix(m[4], "installed"), ",")
		return strings.TrimSpace(m[2] + " " + m[3] + " " + flags)
	}},
	// pip freeze: `Flask==3.0.3`
	{lazyre.New(`^([^\s=:]+)==(\S+)$`), func(m []string) string { return m[2] }},
	// gem list: `rspec (3.13.2)`, `json (default: 2.9.1)`, `rake (13.0.6, 12.3.3)`
	{lazyre.New(`^(\S+) \((?:default: )?([^)]+)\)$`), func(m []string) string { return strings.ReplaceAll(m[2], ", ", ",") }},
	// pip list, pacman -Q, brew list --versions, go list -m all
	{lazyre.New(`^([^\s:=]+)\s+(\S+)((?:\s+\S+)*)$`), func(m []string) string {
		return strings.Join(strings.Fields(m[2]+" "+m[3]), " ")
	}},
}

// Compress rewrites lines; lines that are no package pass through in place.
func Compress(lines []string) []string {
	var out, names []string // names: the run of packages sharing key
	var key string
	flush := func() {
		switch len(names) {
		case 0:
		case 1:
			out = append(out, names[0]+" "+key)
		default:
			out = append(out, key+": "+strings.Join(names, " "))
		}
		names = nil
	}
	for _, l := range lines {
		if keepHeader(l) {
			flush()
			out = append(out, strings.Join(strings.Fields(l), " "))
			continue
		}
		if noise.Match(l) {
			continue
		}
		name, k, ok := parse(l)
		if !ok {
			flush()
			out = append(out, l)
			continue
		}
		if k != key {
			flush()
		}
		names, key = append(names, name), k
	}
	flush()
	return out
}

// keepHeader: a pip header with more than name and version columns
// (`Package Version Latest Type`) says what the extra values mean.
func keepHeader(l string) bool {
	f := strings.Fields(l)
	return len(f) > 2 && f[0] == "Package" && f[1] == "Version"
}

func parse(l string) (name, key string, ok bool) {
	l = strings.TrimSpace(l)
	for _, f := range forms {
		if m := f.re.FindStringSubmatch(l); m != nil {
			return m[1], f.key(m), true
		}
	}
	return "", "", false
}
