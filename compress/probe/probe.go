// Package probe measures what an optimization kept: it pulls facts an agent
// may need out of the original text (error lines, artifacts such as paths,
// URLs and hashes, key=number pairs) and checks each one is still readable in
// the optimized text. Optimizers regroup and reformat (diagnostics by file,
// listings by directory, JSON as tables, path legends), so a fact counts as
// kept when its words survive, not only its exact bytes.
package probe

import (
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/compress/legend"
	"github.com/gascoigne-ai/gascoigne-cli/lazyre"
)

// Tally counts the facts of one kind and lists the lost ones.
type Tally struct {
	Total int      `json:"total"`
	Lost  []string `json:"lost,omitempty"`
}

// Kept is the number of facts that survived.
func (t Tally) Kept() int { return t.Total - len(t.Lost) }

// Add merges u into t.
func (t *Tally) Add(u Tally) {
	t.Total += u.Total
	t.Lost = append(t.Lost, u.Lost...)
}

// Report is the outcome per kind of fact.
type Report struct {
	Errors    Tally `json:"errors"`
	Artifacts Tally `json:"artifacts"`
	Numerics  Tally `json:"numerics"`
}

// Add merges r2 into r.
func (r *Report) Add(r2 Report) {
	r.Errors.Add(r2.Errors)
	r.Artifacts.Add(r2.Artifacts)
	r.Numerics.Add(r2.Numerics)
}

const (
	minLen     = 4
	maxErrLine = 160
)

var (
	urlRe     = lazyre.New(`https?://[^\s"'<>)\]]+`)
	pathRe    = lazyre.New(`(?:~/|\.{1,2}/|/)?(?:[\w.@-]+/){2,}[\w.@-]+`)
	hexRe     = lazyre.New(`\b[0-9a-f]*[a-f][0-9a-f]*\b`)
	uuidRe    = lazyre.New(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	numericRe = lazyre.New(`([A-Za-z_][\w.-]{0,24})"?[ =:]{1,3}(\d+(?:\.\d+)?)\b`)
	fileRe    = lazyre.New(`[\w./@-]*\w\.\w+`)
	// Punctuation that reformatting rewrites; path, URL and hash characters stay.
	punctRe = lazyre.New(`[^\pL\pN_./-]+`)
)

// Run checks which facts of original survive in optimized.
func Run(original, optimized string) Report {
	h := newHaystack(optimized)
	original = compress.StripANSI(original)
	var r Report
	for _, e := range errorLines(original) {
		r.Errors.check(e, h.hasText(e))
	}
	for _, a := range artifacts(original) {
		r.Artifacts.check(a, h.hasArtifact(a))
	}
	for _, n := range numerics(original) {
		r.Numerics.check(n[0], h.hasText(n[0]) || h.hasWord(n[1]) && h.hasWord(n[2]))
	}
	return r
}

func (t *Tally) check(fact string, kept bool) {
	t.Total++
	if !kept {
		t.Lost = append(t.Lost, fact)
	}
}

// errorLines are the distinct lines with error vocabulary, cut to a prefix:
// long lines are capped by optimizers, their start carries the meaning.
// File names don't count as vocabulary (panic.go, errors_test.go), and JSON
// lines are left to the data engines: their keys and timestamps are not facts.
func errorLines(text string) []string {
	var out []string
	for _, l := range compress.Lines(text) {
		l = strings.TrimSpace(l)
		if len(l) < minLen || strings.HasPrefix(l, "{") || !compress.IsError(fileRe.ReplaceAllString(l, "")) {
			continue
		}
		if len(l) > maxErrLine {
			l = strings.ToValidUTF8(l[:maxErrLine], "")
		}
		out = append(out, l)
	}
	return distinct(out)
}

func artifacts(text string) []string {
	var out []string
	for _, re := range []*lazyre.Regexp{urlRe, pathRe, uuidRe} {
		out = append(out, re.FindAllString(text, -1)...)
	}
	for _, m := range hexRe.FindAllString(text, -1) {
		if isHash(m) {
			out = append(out, m)
		}
	}
	return distinct(slices.DeleteFunc(out, func(s string) bool { return len(s) < minLen }))
}

// numerics returns [match, key, number] triples.
func numerics(text string) [][]string {
	var out [][]string
	seen := map[string]bool{}
	for _, m := range numericRe.FindAllStringSubmatch(text, -1) {
		if len(m[0]) < minLen || seen[m[0]] {
			continue
		}
		seen[m[0]] = true
		out = append(out, m)
	}
	return out
}

// isHash: 7–64 hex digits mixing letters and digits (words like "defaced" are not).
func isHash(s string) bool {
	return len(s) >= 7 && len(s) <= 64 && strings.ContainsAny(s, "abcdef") && strings.ContainsAny(s, "0123456789") &&
		strings.Trim(s, "0123456789abcdef") == ""
}

func distinct(s []string) []string {
	seen := map[string]bool{}
	return slices.DeleteFunc(s, func(x string) bool {
		if seen[x] {
			return true
		}
		seen[x] = true
		return false
	})
}

// haystack is the optimized text with path legends expanded, raw and
// normalized, plus its word set.
type haystack struct {
	raw, norm string
	words     map[string]bool
}

func newHaystack(text string) *haystack {
	text = legend.Expand(compress.StripANSI(text))
	h := &haystack{raw: text, norm: normalize(text), words: map[string]bool{}}
	for _, w := range strings.Fields(h.norm) {
		h.words[w] = true
		// Paths split into segments too, so a file listed under its
		// directory still counts.
		for _, s := range strings.Split(w, "/") {
			h.words[s] = true
		}
	}
	return h
}

func normalize(s string) string {
	return strings.Join(strings.Fields(punctRe.ReplaceAllString(s, " ")), " ")
}

func (h *haystack) hasWord(w string) bool { return h.words[normalize(w)] }

// hasText: exact, normalized, or every word present (regrouped lines).
func (h *haystack) hasText(s string) bool {
	if strings.Contains(h.raw, s) {
		return true
	}
	n := normalize(s)
	if n == "" || strings.Contains(h.norm, n) {
		return true
	}
	for _, w := range strings.Fields(n) {
		if !h.words[w] {
			return false
		}
	}
	return true
}

// hasArtifact: verbatim, a hash by its short form, or a path whose segments
// all appear (listings grouped by directory).
func (h *haystack) hasArtifact(a string) bool {
	if strings.Contains(h.raw, a) {
		return true
	}
	if isHash(a) {
		return strings.Contains(h.raw, a[:7])
	}
	for _, s := range strings.Split(strings.Trim(a, "/"), "/") {
		if s != "" && s != "." && s != ".." && s != "~" && !h.words[s] {
			return false
		}
	}
	return true
}
