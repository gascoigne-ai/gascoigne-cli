// Package cloudoptimizer is the cloud family engine for the AWS, Google
// Cloud and Azure CLIs: JSON responses lose pagination, request metadata,
// etags and default states before being crushed (compress/jsonx), YAML
// (gcloud describe) is pruned the same way, tables are compacted, and object
// listings (aws s3 ls) are summarized.
package cloudoptimizer

import (
	"context"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/jsonx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/yamlx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	query := sample.Terms(p.Attrs[payload.AttrQuery])
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyCloud {
			continue
		}
		out, err := o.block(b, query)
		if err != nil {
			return err
		}
		p.Blocks[i].Text = out
	}
	return nil
}

func (o *Optimizer) block(b payload.Block, query []string) (string, error) {
	text := strings.TrimSpace(b.Text)
	if detect.Dialect(b.Type) == "aws-s3" {
		return compress.Join(o.s3(compress.Lines(compress.StripANSI(b.Text)))), nil
	}
	switch {
	case strings.HasPrefix(text, "{") || strings.HasPrefix(text, "["):
		values, err := jsonx.Decode(text)
		if err != nil {
			return "", err
		}
		for i := range values {
			values[i] = reshape(prune(values[i]))
		}
		return jsonx.Render(values, false, &o.cfg.JSON, query)
	}
	if t, ok := table.Parse(compress.Lines(compress.StripANSI(b.Text))); ok {
		return compress.Join(table.Render(t, table.ForQuery(o.cfg.MaxRows, o.cfg.MaxCell, query))), nil
	}
	if docs, err := yamlx.Docs(b.Text); err == nil && structured(docs) {
		for _, d := range docs {
			yamlx.Prune(d, yamlRules)
		}
		return yamlx.Encode(docs)
	}
	return b.Text, nil
}

// structured reports YAML that is a mapping or sequence, not plain text
// (which parses as one scalar).
func structured(docs []*yaml.Node) bool {
	if len(docs) == 0 || len(docs[0].Content) == 0 {
		return false
	}
	k := docs[0].Content[0].Kind
	return k == yaml.MappingNode || k == yaml.SequenceNode
}
