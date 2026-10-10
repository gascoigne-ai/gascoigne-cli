// Package k8soptimizer is the Kubernetes family engine: manifests (kubectl
// get -o yaml/json, helm template, kustomize) lose bookkeeping and default
// fields, long lists keep unhealthy items, describe output drops empty and
// opaque fields while keeping warnings, tables (get, events, helm list) go
// through the table engine, helm NOTES are capped.
package k8soptimizer

import (
	"context"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/compress/rows"
	"github.com/lukaszb/gascoigne/compress/table"
	"github.com/lukaszb/gascoigne/compress/yamlx"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/lazyre"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

func (o *Optimizer) Optimize(_ context.Context, p *payload.Payload) error {
	topts := table.Options{MaxRows: o.cfg.MaxRows, MaxCell: o.cfg.MaxCell,
		Rows: rows.Options{HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2, RelevanceK: 3, Query: rows.Terms(p.Attrs[payload.AttrQuery])}}
	for i, b := range p.Blocks {
		if detect.Family(b.Type) != detect.FamilyK8s {
			continue
		}
		lines := compress.Lines(compress.StripANSI(b.Text))
		var out []string
		switch detect.Dialect(b.Type) {
		case "manifest":
			text, err := o.manifests(b.Text)
			if err != nil {
				return err
			}
			p.Blocks[i].Text = text
			continue
		case "describe":
			out = describe(lines)
		case "events":
			out = events(lines, topts)
		case "helm":
			out = o.helm(lines, topts)
		default: // get and other tables
			if t, ok := table.Parse(lines); ok {
				out = table.Render(t, topts)
			} else {
				out = lines
			}
		}
		p.Blocks[i].Text = compress.Join(out)
	}
	return nil
}

// manifests prunes every document; lists keep unhealthy items and ends.
func (o *Optimizer) manifests(text string) (string, error) {
	docs, err := yamlx.Docs(text)
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		yamlx.Prune(d, rules)
		o.sampleList(d)
	}
	return yamlx.Encode(docs)
}

var unhealthy = lazyre.New(`CrashLoopBackOff|ImagePullBackOff|ErrImagePull|OOMKilled|Error|Failed|Pending|Evicted|Unknown|ready: false|NotReady|Unhealthy`)

// sampleList keeps MaxItems of a kind: List: unhealthy ones first, then ends.
func (o *Optimizer) sampleList(doc *yaml.Node) {
	root := doc
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value != "items" || len(root.Content[i+1].Content) <= o.cfg.MaxItems {
			continue
		}
		items := root.Content[i+1].Content
		keep := map[int]bool{}
		for j, it := range items {
			b, _ := yaml.Marshal(it)
			if unhealthy.Match(b) {
				keep[j] = true
			}
		}
		for j := 0; len(keep) < o.cfg.MaxItems && j < len(items); j++ {
			keep[j] = true
		}
		var out []*yaml.Node
		gap := 0
		for j, it := range items {
			if !keep[j] {
				gap++
				continue
			}
			if gap > 0 {
				out = append(out, &yaml.Node{Kind: yaml.ScalarNode, Value: compress.Omitted(gap, "items")})
				gap = 0
			}
			out = append(out, it)
		}
		if gap > 0 {
			out = append(out, &yaml.Node{Kind: yaml.ScalarNode, Value: compress.Omitted(gap, "items")})
		}
		root.Content[i+1].Content = out
	}
}

var (
	describeNoise = lazyre.New(`^\s*(Container ID|Image ID|Controlled By|Priority|Service Account|QoS Class|Node-Selectors|SeccompProfile|IPs|IP Family Policy|IP Families|Session Affinity|Internal Traffic Policy|Topology Spread Constraints|Priority Class Name|Runtime Class Name|Annotations):|:\s+<none>\s*$|^\s*(TokenExpirationSeconds|ConfigMapName|ConfigMapOptional|DownwardAPI|Optional):|^\s+node\.kubernetes\.io/(not-ready|unreachable):NoExecute op=Exists for 300s`)
	apiAccess     = lazyre.New(`^(\s*)kube-api-access-\S+:$`)
	eventRow      = lazyre.New(`^\s+(Normal|Warning)\s+(\S+)\s+`)
)

// describe drops empty, opaque and default fields; Normal events with the
// same reason fold, Warning events all stay.
func describe(lines []string) []string {
	var out []string
	skipIndent := -1
	normal := map[string]int{}
	firstNormal := map[string]int{}
	for _, l := range lines {
		indent := len(l) - len(strings.TrimLeft(l, " "))
		if skipIndent >= 0 {
			if strings.TrimSpace(l) != "" && indent > skipIndent {
				continue
			}
			skipIndent = -1
		}
		if m := apiAccess.FindStringSubmatch(l); m != nil {
			skipIndent = len(m[1])
			continue
		}
		if describeNoise.MatchString(l) {
			continue
		}
		if m := eventRow.FindStringSubmatch(l); m != nil && m[1] == "Normal" {
			if _, seen := firstNormal[m[2]]; seen {
				normal[m[2]]++
				continue
			}
			firstNormal[m[2]] = len(out)
		}
		out = append(out, strings.TrimRight(l, " "))
	}
	for reason, n := range normal {
		out[firstNormal[reason]] += fmt.Sprintf(" (×%d)", n+1)
	}
	return compress.SqueezeBlank(out)
}

// events keeps every Warning and samples the rest.
func events(lines []string, o table.Options) []string {
	t, ok := table.Parse(lines)
	if !ok {
		return lines
	}
	typeCol := -1
	for i, h := range t.Header {
		if strings.EqualFold(h, "TYPE") {
			typeCol = i
		}
	}
	if typeCol >= 0 {
		var warn, rest [][]string
		for _, r := range t.Rows {
			if r[typeCol] == "Warning" {
				warn = append(warn, r)
			} else {
				rest = append(rest, r)
			}
		}
		o.MaxRows = max(o.MaxRows-len(warn), 3)
		t.Rows = rest
		out := table.Render(t, o)
		w := table.Table{Header: t.Header, Rows: warn}
		if len(warn) > 0 {
			o.MaxRows = len(warn)
			return append(append(out, "warnings:"), table.Render(w, o)...)
		}
		return out
	}
	return table.Render(t, o)
}

var notesStart = lazyre.New(`^NOTES:\s*$`)

// helm caps NOTES (chart instructions); tables (list, history) are compacted.
func (o *Optimizer) helm(lines []string, topts table.Options) []string {
	if t, ok := table.Parse(lines); ok && len(t.Rows) >= 2 {
		return table.Render(t, topts)
	}
	var out []string
	notes := -1
	for _, l := range lines {
		if notesStart.MatchString(l) {
			notes = 0
			out = append(out, l)
			continue
		}
		if notes >= 0 {
			notes++
			if notes > o.cfg.NotesLines {
				continue
			}
		}
		out = append(out, l)
	}
	if notes > o.cfg.NotesLines {
		out = append(out, compress.Omitted(notes-o.cfg.NotesLines, "notes lines"))
	}
	return compress.SqueezeBlank(out)
}
