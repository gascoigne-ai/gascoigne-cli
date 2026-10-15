// Package dataoptimizer is the data family engine for YAML, CSV/TSV, XML and
// TOML (JSON has its own optimizer): YAML goes through the JSON crusher when
// that is smaller, CSV through the table engine, XML keeps the ends of long
// sibling runs plus failures, and TOML lockfiles fold to one line per entry.
package dataoptimizer

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/yamlx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
	"github.com/gascoigne-ai/gascoigne-cli/tokens"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	for i, b := range p.Blocks {
		var out string
		var err error
		switch b.Type {
		case detect.YAML:
			out, err = o.yaml(b.Text, query)
		case detect.CSV:
			out = o.csv(b.Text, query)
		case detect.XML:
			out, err = o.xml(b.Text)
		case detect.TOML:
			out = o.toml(b.Text)
		case detect.IPYNB:
			out, err = o.notebook(b.Text)
		default:
			continue
		}
		if err != nil {
			return err
		}
		p.Blocks[i].Text = out
	}
	return nil
}

// yaml crushes documents as JSON, kept only when it beats the original.
func (o *Optimizer) yaml(text string, query []string) (string, error) {
	docs, err := yamlx.Docs(text)
	if err != nil {
		return "", err
	}
	var values []any
	for _, d := range docs {
		var v any
		if err := d.Decode(&v); err != nil {
			return "", err
		}
		// Through JSON for the crusher's value types.
		b, err := json.Marshal(v)
		if err != nil {
			return text, nil // maps with non-string keys: leave as is
		}
		decoded, err := jsonx.Decode(string(b))
		if err != nil {
			return "", err
		}
		values = append(values, decoded...)
	}
	out, err := jsonx.Render(values, len(values) > 1, &o.cfg.JSON, query)
	if err != nil || tokens.Estimate(out) >= tokens.Estimate(text) {
		return text, err
	}
	return out, nil
}

// csv renders rows through the table engine.
func (o *Optimizer) csv(text string, query []string) string {
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	if first, _, _ := strings.Cut(text, "\n"); strings.Count(first, "\t") > strings.Count(first, ",") {
		r.Comma = '\t'
	} else if strings.Count(first, ";") > strings.Count(first, ",") {
		r.Comma = ';'
	}
	records, err := r.ReadAll()
	if err != nil || len(records) < 2 {
		return text
	}
	t := table.Table{Header: records[0], Rows: records[1:]}
	return compress.Join(table.Render(t, table.Options{MaxRows: o.cfg.MaxRows, MaxCell: o.cfg.MaxCell,
		Rows: sample.Options{HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2, RelevanceK: 3, Query: query}}))
}
