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

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/sample"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/yamlx"
	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
	"github.com/gascoigne-ai/gascoigne-cli/internal/pcomb"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct {
	component.Nop
	cfg *Config
}

// handlers compress one dialect; others are tables. Manifests are YAML
// documents, not lines, and go first.
var handlers = map[string]func(opt *Optimizer, lines []string, topts table.Options) []string{
	"describe": func(_ *Optimizer, lines []string, _ table.Options) []string { return describe(lines) },
	"events":   func(_ *Optimizer, lines []string, topts table.Options) []string { return events(lines, topts) },
	"apply":    func(_ *Optimizer, lines []string, _ table.Options) []string { return apply(lines) },
	"rollout":  func(_ *Optimizer, lines []string, _ table.Options) []string { return rollout(lines) },
	"helm":     (*Optimizer).helm,
}

func tableOf(_ *Optimizer, lines []string, topts table.Options) []string {
	if out, ok := table.Compress(lines, topts); ok {
		return out
	}
	return lines
}

func (o *Optimizer) Optimize(_ context.Context, data *payload.Payload) error {
	topts := o.cfg.Options(sample.Terms(data.Attrs[payload.AttrQuery]))
	return data.RewriteFamily(func(block payload.Block) (string, error) {
		if detect.Dialect(block.Type) == "manifest" {
			return o.manifests(block.Text)
		}
		lines := compress.CleanLines(block.Text)
		handle, ok := handlers[detect.Dialect(block.Type)]
		if !ok {
			handle = tableOf // get and other tables
		}
		out := handle(o, lines, topts)
		return compress.Join(out), nil
	}, detect.FamilyK8s)
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
		root.Content[i+1].Content = o.sampleItems(root.Content[i+1].Content)
	}
}

// sampleItems keeps MaxItems of items: the unhealthy ones, then the first
// ones to fill up; a marker stands for each run of dropped items.
func (o *Optimizer) sampleItems(items []*yaml.Node) []*yaml.Node {
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
			out = append(out, omittedItems(gap))
			gap = 0
		}
		out = append(out, it)
	}
	if gap > 0 {
		out = append(out, omittedItems(gap))
	}
	return out
}

// omittedItems is the marker node for n dropped items.
func omittedItems(count int) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: compress.Omitted(count, "items")}
}

var (
	describeNoise = pcomb.Rules{
		{
			Name: "empty or default field",
			Match: pcomb.Trimmed(pcomb.Prefix("Container ID:", "Image ID:", "Controlled By:", "Priority:", "Service Account:", "QoS Class:", "Node-Selectors:",
				"SeccompProfile:", "IPs:", "IP Family Policy:", "IP Families:", "Session Affinity:", "Internal Traffic Policy:", "Topology Spread Constraints:",
				"Priority Class Name:", "Runtime Class Name:", "Annotations:")),
			Yes: []string{"Container ID:  containerd://3f9a", "    Image ID:       docker.io/library/nginx@sha256:abc", "Priority:       0", "  Annotations:  <none>"},
			No:  []string{"Priority", "Name:  x", "Image:  nginx"},
		},
		{
			Name:  "none value",
			Match: pcomb.Re(`:\s+<none>\s*$`),
			Yes:   []string{"Labels:  <none>", "    Limits:  <none>  ", "x: <none>"},
			No:    []string{"Labels:  <none> x", "Labels:<none>", "<none>"},
		},
		{
			Name:  "projected volume field",
			Match: pcomb.Trimmed(pcomb.Prefix("TokenExpirationSeconds:", "ConfigMapName:", "ConfigMapOptional:", "DownwardAPI:", "Optional:")),
			Yes:   []string{"    TokenExpirationSeconds:  3607", "    ConfigMapName:           kube-root-ca.crt", "    ConfigMapOptional:       <nil>", "    DownwardAPI:             true", "  Optional:  false"},
			No:    []string{"Optional", "Mode:  420"},
		},
		{
			Name:  "default toleration",
			Match: pcomb.Indented(pcomb.Prefix("node.kubernetes.io/not-ready:NoExecute op=Exists for 300s", "node.kubernetes.io/unreachable:NoExecute op=Exists for 300s")),
			Yes:   []string{"                 node.kubernetes.io/not-ready:NoExecute op=Exists for 300s", "                 node.kubernetes.io/unreachable:NoExecute op=Exists for 300s"},
			No:    []string{"node.kubernetes.io/not-ready:NoExecute op=Exists for 300s", "                 node.kubernetes.io/memory-pressure:NoSchedule op=Exists"},
		},
	}
	apiAccess = lazyre.New(`^(\s*)kube-api-access-\S+:$`)
	eventRow  = lazyre.New(`^\s+(Normal|Warning)\s+(\S+)\s+`)
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
		if describeNoise.Match(l) {
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
func events(lines []string, options table.Options) []string {
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
		options.MaxRows = max(options.MaxRows-len(warn), 3)
		t.Rows = rest
		out := table.Render(t, options)
		w := table.Table{Header: t.Header, Rows: warn}
		if len(warn) > 0 {
			options.MaxRows = len(warn)
			return append(append(out, "warnings:"), table.Render(w, options)...)
		}
		return out
	}
	return table.Render(t, options)
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
