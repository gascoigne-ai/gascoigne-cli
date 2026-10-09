package gitoptimizer

import (
	"fmt"
	"path"
	"strings"
)

var sections = map[string]string{
	"Changes to be committed:":       "staged",
	"Changes not staged for commit:": "unstaged",
	"Untracked files:":               "untracked",
	"Unmerged paths:":                "conflicts",
}

var states = map[string]string{
	"modified":        "M",
	"new file":        "A",
	"deleted":         "D",
	"renamed":         "R",
	"copied":          "C",
	"typechange":      "T",
	"both modified":   "U",
	"both added":      "U",
	"both deleted":    "U",
	"added by us":     "U",
	"added by them":   "U",
	"deleted by us":   "U",
	"deleted by them": "U",
}

// status rewrites long-form git status into one section per state with
// short codes, dropping "(use ...)" hints.
func status(lines []string) []string {
	var out []string
	section := ""
	var entries []string
	flush := func() {
		if section != "" && len(entries) > 0 {
			out = append(out, section+":")
			out = append(out, groupByDir(entries)...)
		}
		entries = nil
	}
	for _, l := range lines {
		t := strings.TrimSpace(l)
		switch {
		case t == "" || strings.HasPrefix(t, "(use ") || strings.HasPrefix(t, "(commit or discard"):
		case sections[t] != "":
			flush()
			section = sections[t]
		case strings.HasPrefix(l, "\t") && section != "":
			entries = append(entries, entry(t, section))
		default:
			flush()
			section = ""
			out = append(out, t)
		}
	}
	flush()
	return out
}

func entry(t, section string) string {
	if section == "untracked" {
		return "? " + t
	}
	state, file, ok := strings.Cut(t, ":")
	code, known := states[state]
	if !ok || !known {
		return t
	}
	return code + " " + strings.TrimSpace(file)
}

// groupByDir folds 3+ entries with the same code and directory into one line.
func groupByDir(entries []string) []string {
	type key struct{ code, dir string }
	groups := map[key][]string{}
	var order []key
	for _, e := range entries {
		code, file, _ := strings.Cut(e, " ")
		k := key{code, path.Dir(file)}
		if strings.Contains(file, " -> ") || strings.HasSuffix(file, "/") {
			k.dir = "" // renames and untracked dirs stay alone
		}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], file)
	}
	var out []string
	for _, k := range order {
		files := groups[k]
		if len(files) < 3 || k.dir == "" || k.dir == "." {
			for _, f := range files {
				out = append(out, "  "+k.code+" "+f)
			}
			continue
		}
		names := make([]string, len(files))
		for i, f := range files {
			names[i] = path.Base(f)
		}
		out = append(out, fmt.Sprintf("  %s %s/{%s}", k.code, k.dir, strings.Join(names, ",")))
	}
	return out
}
