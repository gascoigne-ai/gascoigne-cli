package dataoptimizer

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
)

// notebook renders a Jupyter notebook the way it reads in an editor: cell
// sources as they are, text outputs capped, images and widget state named
// by size, metadata dropped. Error tracebacks stay whole.
func (o *Optimizer) notebook(text string) (string, error) {
	var nb struct {
		Cells []struct {
			Type    string            `json:"cell_type"`
			Source  source            `json:"source"`
			Count   *int              `json:"execution_count"`
			Outputs []json.RawMessage `json:"outputs"`
		} `json:"cells"`
		Metadata struct {
			Kernel struct {
				Name string `json:"name"`
			} `json:"kernelspec"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(text), &nb); err != nil {
		return "", fmt.Errorf("notebook: %w", err)
	}
	var out []string
	if k := nb.Metadata.Kernel.Name; k != "" {
		out = append(out, "# notebook, kernel "+k)
	}
	for i, c := range nb.Cells {
		head := fmt.Sprintf("# %%%% [%d] %s", i+1, c.Type)
		if c.Count != nil {
			head += fmt.Sprintf(" In[%d]", *c.Count)
		}
		out = append(out, head)
		out = append(out, compress.Lines(string(c.Source))...)
		for _, raw := range c.Outputs {
			out = append(out, o.cellOutput(raw)...)
		}
	}
	return compress.Join(out), nil
}

// source is a cell's text, stored as one string or a list of lines.
type source string

func (s *source) UnmarshalJSON(b []byte) error {
	var parts []string
	if err := json.Unmarshal(b, &parts); err == nil {
		*s = source(strings.Join(parts, ""))
		return nil
	}
	var one string
	err := json.Unmarshal(b, &one)
	*s = source(one)
	return err
}

// cellOutput renders one output: stream and plain text capped, rich data
// by its most readable type, errors with their traceback.
func (o *Optimizer) cellOutput(raw json.RawMessage) []string {
	var out struct {
		Type      string                     `json:"output_type"`
		Name      string                     `json:"name"`
		Text      source                     `json:"text"`
		Data      map[string]json.RawMessage `json:"data"`
		EName     string                     `json:"ename"`
		EValue    string                     `json:"evalue"`
		Traceback []string                   `json:"traceback"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return []string{fmt.Sprintf("# out: [unreadable output, %d bytes]", len(raw))}
	}
	switch out.Type {
	case "stream":
		return append([]string{"# out: " + out.Name}, o.capped(string(out.Text))...)
	case "error":
		lines := []string{fmt.Sprintf("# error: %s: %s", out.EName, out.EValue)}
		for _, t := range out.Traceback {
			lines = append(lines, compress.Lines(compress.StripANSI(t))...)
		}
		return lines
	}
	lines := []string{"# out:"}
	var plain source
	if t, ok := out.Data["text/plain"]; ok && json.Unmarshal(t, &plain) == nil {
		lines = append(lines, o.capped(string(plain))...)
	}
	return append(lines, binaries(out.Data)...)
}

// capped keeps head and tail of a long text output.
func (o *Optimizer) capped(text string) []string {
	lines := compress.Lines(text)
	n := o.cfg.MaxOutputLines
	if len(lines) <= n {
		return lines
	}
	return compress.HeadTail(lines, n*2/3, n-n*2/3)
}

// binaries names the other representations of an output by type and size
// (as stored), in a stable order.
func binaries(data map[string]json.RawMessage) []string {
	var out []string
	for typ, v := range data {
		if typ == "text/plain" {
			continue
		}
		out = append(out, fmt.Sprintf("[%s %s]", typ, size(len(v))))
	}
	slices.Sort(out)
	return out
}

func size(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%d KB", n/1024)
}
