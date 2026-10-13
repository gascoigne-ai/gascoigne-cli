// Package embedjsonoptimizer crushes JSON embedded in text: response bodies
// in logs, payloads in test failures. Such blocks are typed by their text, so
// the JSON engine never sees the arrays of records that cost most of their
// tokens. Each value holding one is replaced by its crushed form in place.
package embedjsonoptimizer

import (
	"context"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

var legend = "// " + jsonx.Legend + "\n"

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	for i, b := range p.Blocks {
		if o.wants(b.Type) {
			p.Blocks[i].Text = o.crush(b.Text, query)
		}
	}
	return nil
}

func (o *Optimizer) wants(typ string) bool {
	return slices.Contains(o.cfg.Types, typ) || slices.Contains(o.cfg.Types, detect.Family(typ))
}

func (o *Optimizer) crush(text string, query []string) string {
	spans := jsonx.Spans(text)
	if len(spans) == 1 && strings.TrimSpace(text) == text[spans[0][0]:spans[0][1]] {
		return text // a whole JSON block is not embedded
	}
	var out strings.Builder
	last, table := 0, false
	for _, s := range spans {
		crushed, ok := o.value(text[s[0]:s[1]], query)
		if !ok {
			continue
		}
		out.WriteString(text[last:s[0]])
		out.WriteString(crushed)
		last, table = s[1], table || strings.Contains(crushed, `"_cols":`)
	}
	if last == 0 {
		return text
	}
	out.WriteString(text[last:])
	if table && !strings.HasPrefix(text, legend) {
		return legend + out.String()
	}
	return out.String()
}

// value crushes one JSON value holding an array of records; ok only when that
// made it smaller. Scalars and plain lists are cheap and stay as written.
func (o *Optimizer) value(span string, query []string) (string, bool) {
	values, err := jsonx.Decode(span)
	if err != nil || !records(values[0]) {
		return "", false
	}
	out, err := jsonx.Render(values, false, &o.cfg.JSON, query)
	if err != nil {
		return "", false
	}
	out = strings.TrimSuffix(strings.TrimPrefix(out, legend), "\n")
	return out, tokens.Estimate(out) < tokens.Estimate(span)
}

// records reports an array of at least two items, mostly objects, anywhere in v.
func records(v any) bool {
	switch v := v.(type) {
	case []any:
		objects := 0
		for _, e := range v {
			if _, ok := e.(map[string]any); ok {
				objects++
			}
		}
		if len(v) >= 2 && objects*5 >= len(v)*4 {
			return true
		}
		return slices.ContainsFunc(v, records)
	case map[string]any:
		for _, e := range v {
			if records(e) {
				return true
			}
		}
	}
	return false
}
