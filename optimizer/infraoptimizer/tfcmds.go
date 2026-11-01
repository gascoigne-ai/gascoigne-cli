package infraoptimizer

import (
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	tfBoxStart = lazyre.New(`^╷\s*$`)
	tfBoxEnd   = lazyre.New(`^╵\s*$`)
	tfLocation = lazyre.New(`^\s*on (\S+) line (\d+)`)
	tfWith     = lazyre.New(`^\s*with .*,$`)
	tfSnippet  = lazyre.New(`^\s+\d+:`)
)

// tfBoxes drops the box drawing around diagnostics. A warning that repeats
// (same text, other place) is written once with its places; errors stay.
func tfBoxes(lines []string) []string {
	var out, box []string
	inBox := false
	first := map[string]int{} // warning text -> index of its first line in out
	places := map[string][]string{}
	for _, l := range lines {
		switch {
		case !inBox && tfBoxStart.MatchString(l):
			inBox, box = true, nil
		case inBox && tfBoxEnd.MatchString(l):
			inBox = false
			out = tfEmitBox(out, box, first, places)
		case inBox:
			box = append(box, strings.TrimPrefix(strings.TrimPrefix(l, "│"), " "))
		default:
			out = append(out, l)
		}
	}
	if inBox { // cut-off output
		out = append(out, box...)
	}
	for text, at := range first {
		if p := places[text]; len(p) > 0 {
			out[at] += fmt.Sprintf(" (×%d, also %s)", len(p)+1, strings.Join(p, ", "))
		}
	}
	return out
}

func tfEmitBox(out, box []string, first map[string]int, places map[string][]string) []string {
	for len(box) > 0 && strings.TrimSpace(box[0]) == "" {
		box = box[1:]
	}
	if len(box) == 0 {
		return out
	}
	var text, place string
	for _, l := range box {
		if m := tfLocation.FindStringSubmatch(l); m != nil {
			place = m[1] + ":" + m[2]
		} else if !tfSnippet.MatchString(l) && !tfWith.MatchString(l) {
			text += l + "\n"
		}
	}
	warning := strings.HasPrefix(box[0], "Warning:")
	if _, seen := first[text]; seen && warning {
		places[text] = append(places[text], place)
		return out
	}
	if len(out) > 0 && out[len(out)-1] != "" { // boxes were set apart
		out = append(out, "")
	}
	if warning {
		first[text] = len(out)
	}
	return append(out, box...)
}

var (
	tfInitNoise = lazyre.New(`^(Initializing (the backend|modules|provider plugins)\.\.\.|use this backend unless|- (Finding|Installing) )`)
	// First line of boilerplate paragraphs, dropped up to the next blank line.
	tfInitBoiler = lazyre.New(`^((Terraform|OpenTofu) has (created a lock file|made some changes to the provider dependency)|You may now begin working with|If you ever set or change modules)`)
	tfBackendOK  = lazyre.New(`^(Successfully configured the backend "[^"]+")!`)
	tfInstalled  = lazyre.New(`^- (?:Installed|Using previously-installed) (\S+) (v\S+)(?: \((.*)\))?$`)
)

// terraformInit keeps module sources, errors and the final status; the
// providers go on one line.
func terraformInit(lines []string) []string {
	var out, providers []string
	at := -1
	skip := false
	for _, l := range tfBoxes(lines) {
		if strings.TrimSpace(l) == "" {
			skip = false
			out = append(out, l)
			continue
		}
		if m := tfInstalled.FindStringSubmatch(l); m != nil {
			if at < 0 {
				at = len(out)
				out = append(out, "")
			}
			name := m[1] + " " + m[2]
			if strings.HasPrefix(m[3], "unauthenticated") || strings.HasPrefix(m[3], "self-signed") {
				name += " (" + m[3] + ")"
			}
			providers = append(providers, name)
			continue
		}
		if m := tfBackendOK.FindStringSubmatch(l); m != nil {
			l = m[1]
		}
		switch {
		case tfInitNoise.MatchString(l):
		case tfInitBoiler.MatchString(l):
			skip = true
		case !skip:
			out = append(out, l)
		}
	}
	if at >= 0 {
		out[at] = "providers: " + strings.Join(providers, ", ")
	}
	return out
}

// tfAddress is a resource address as `state list` prints it: module path,
// optional data., type, name, optional [index].
var tfAddress = lazyre.New(`^((?:module\.[\w-]+(?:\[[^\]]*\])?\.)*)((?:data\.)?[\w-]+\.[\w-]+)(?:\[(.*)\])?$`)

type tfItem struct {
	name string
	keys []string
}

// terraformState groups `state list` addresses by module and folds
// count/for_each instances: `aws_subnet.private[×3]`. Other lines (errors)
// stay in front.
func terraformState(lines []string) []string {
	var scopes, out []string
	items := map[string][]*tfItem{}
	for _, l := range lines {
		m := tfAddress.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			out = append(out, l)
			continue
		}
		scope := cmpScope(m[1])
		if _, ok := items[scope]; !ok {
			scopes = append(scopes, scope)
		}
		list := items[scope]
		if n := len(list); n > 0 && m[3] != "" && list[n-1].name == m[2] {
			list[n-1].keys = append(list[n-1].keys, m[3])
			continue
		}
		it := &tfItem{name: m[2]}
		if m[3] != "" {
			it.keys = []string{m[3]}
		}
		items[scope] = append(list, it)
	}
	for _, s := range scopes {
		parts := make([]string, 0, len(items[s]))
		for _, it := range items[s] {
			parts = append(parts, it.name+tfKeys(it.keys))
		}
		out = append(out, s+": "+strings.Join(parts, ", "))
	}
	return out
}

func cmpScope(module string) string {
	if module == "" {
		return "(root)"
	}
	return strings.TrimSuffix(module, ".")
}

// tfKeys writes instance keys: a few string keys of for_each stay, other
// instances are counted.
func tfKeys(keys []string) string {
	switch {
	case len(keys) == 0:
		return ""
	case len(keys) == 1 || strings.HasPrefix(keys[0], `"`) && len(keys) <= 6:
		return "[" + strings.Join(keys, ",") + "]"
	}
	return fmt.Sprintf("[×%d]", len(keys))
}

// maxOutputList caps a list value of `terraform output`.
const maxOutputList = 20

var (
	tfOutputHead = lazyre.New(`^[\w-]+ = .*([\[{(]|<<-?\w+)$`)
	tfOutputElem = lazyre.New(`^  [^\s\[{(][^\[{(]*$`)
)

// terraformOutput joins list and map values made of plain elements on one
// line; nested values, heredocs and `-raw`/`-json` text stay as printed.
func terraformOutput(lines []string) []string {
	var out []string
	for i := 0; i < len(lines); {
		j := i + 1
		if tfOutputHead.MatchString(lines[i]) {
			for j < len(lines) && strings.HasPrefix(lines[j], " ") {
				j++
			}
			if j < len(lines) && isCloser(lines[j]) {
				j++
			}
		}
		out = append(out, joinOutput(lines[i:j])...)
		i = j
	}
	return out
}

func isCloser(l string) bool { return l != "" && strings.ContainsRune("]})", rune(l[0])) }

// joinOutput writes one entry (header, indented elements, closer) on a line
// when every element is plain.
func joinOutput(entry []string) []string {
	n := len(entry)
	if n < 3 || strings.Contains(entry[0], "<<") || !isCloser(entry[n-1]) {
		return entry
	}
	vals := make([]string, 0, n-2)
	for _, e := range entry[1 : n-1] {
		if !tfOutputElem.MatchString(e) {
			return entry
		}
		vals = append(vals, strings.TrimSuffix(strings.TrimSpace(e), ","))
	}
	more := ""
	if len(vals) > maxOutputList {
		more = fmt.Sprintf(", … +%d", len(vals)-maxOutputList)
		vals = vals[:maxOutputList]
	}
	return []string{entry[0] + strings.Join(vals, ", ") + more + strings.TrimSpace(entry[n-1])}
}
