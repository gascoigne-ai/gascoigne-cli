// Package denseoptimizer elides dense machine-generated lines: minified
// bundles, base64 blobs, data URIs, source maps. Such a line is long and
// nearly free of spaces, costs many tokens and tells an agent nothing beyond
// its start and end, which are kept.
package denseoptimizer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// maxSpaceRatio is the share of spaces below which a line reads as machine
// output: prose and code sit well above it.
const maxSpaceRatio = 0.06

// A delimited list has at least minFields fields, minWordShare of them words
// of up to maxWord bytes.
const (
	minFields    = 8
	minWordShare = 0.8
	maxWord      = 40
)

const codeType = "fs/code"

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	disk := p.Attrs[payload.AttrFile] != "" || detect.ReadsFile(p.Attrs[payload.AttrCmd])
	for i, b := range p.Blocks {
		// Source read from disk is edited by exact text; fetched code is not.
		if o.skip(b.Type) || disk && b.Type == codeType {
			continue
		}
		p.Blocks[i].Text = o.elide(b.Text)
	}
	return nil
}

func (o *Optimizer) skip(typ string) bool {
	return detect.Matches(o.cfg.Skip, typ)
}

func (o *Optimizer) elide(text string) string {
	lines := compress.Lines(text)
	var dense []int
	total := 0
	for i, l := range lines {
		if o.dense(l) {
			dense = append(dense, i)
			total += len(l)
		}
	}
	if total < o.cfg.MinTotal {
		return text
	}
	for _, i := range dense {
		lines[i] = o.cut(lines[i])
	}
	return compress.Join(lines)
}

// dense reports a long, nearly space-free line that is not JSON: tabs mark
// TSV rows and JSON (alone or after a label) belongs to the JSON engine.
func (o *Optimizer) dense(l string) bool {
	if len(l) < o.cfg.MinLine || strings.ContainsRune(l, '\t') {
		return false
	}
	if float64(strings.Count(l, " "))/float64(len(l)) >= maxSpaceRatio {
		return false
	}
	if delimitedList(l) {
		return false
	}
	i := strings.IndexAny(l, "{[")
	return i < 0 || !json.Valid([]byte(l[i:]))
}

// delimitedList reports a line of names joined by one delimiter (`a|b|c`,
// `x,y,z`): most fields are short words, which minified code (calls, braces,
// operators) and base64 (no delimiters) never are. Folding it loses content.
func delimitedList(l string) bool {
	for _, sep := range "|,;" {
		fields := strings.Split(l, string(sep))
		if len(fields) < minFields {
			continue
		}
		words := 0
		for _, f := range fields {
			if wordLike(f) {
				words++
			}
		}
		if float64(words)/float64(len(fields)) >= minWordShare {
			return true
		}
	}
	return false
}

func wordLike(f string) bool {
	f = strings.TrimSpace(f)
	return f != "" && len(f) <= maxWord && !strings.ContainsFunc(f, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.@", r)
	})
}

func (o *Optimizer) cut(l string) string {
	head := runeStart(l, o.cfg.Head)
	tail := runeStart(l, len(l)-o.cfg.Tail)
	return fmt.Sprintf("%s …[%d bytes dense]… %s", l[:head], tail-head, l[tail:])
}

// runeStart moves i back to the start of the rune it falls in.
func runeStart(s string, i int) int {
	i = min(i, len(s))
	for i > 0 && i < len(s) && !utf8.RuneStart(s[i]) {
		i--
	}
	return i
}
