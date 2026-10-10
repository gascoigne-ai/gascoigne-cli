// Package payload is the data model flowing through pipelines.
package payload

import (
	"maps"
	"slices"
	"strings"

	"github.com/lukaszb/gascoigne/tokens"
)

type Kind string

const (
	CmdOutput  Kind = "cmd_output"
	ToolResult Kind = "tool_result"
	Message    Kind = "message"
	JSON       Kind = "json"
)

// Well-known attribute keys.
const (
	AttrFile      = "file"
	AttrCmd       = "cmd"
	AttrTool      = "tool"
	AttrURL       = "url"
	AttrQuery     = "query"       // what the agent looks for; guides lossy selection
	AttrSession   = "session"     // agent session, for per-session stats
	AttrSkip      = "skip_reason" // why nothing was optimized, with facts: set by bypassing filters or no-op optimizers
	AttrRaw       = "raw"
	AttrRawID     = "raw_id"
	AttrRoute     = "route"
	AttrRawTokens = "raw_tokens"
	AttrOutTokens = "out_tokens"
)

type Block struct {
	Kind Kind   `json:"kind"`
	Text string `json:"text"`
	// Type is the detected content type (see package detect); empty until detected.
	Type string `json:"type,omitempty"`
}

type Payload struct {
	Blocks []Block           `json:"blocks"`
	Attrs  map[string]string `json:"attrs,omitempty"`
}

func New(kind Kind, text string) *Payload {
	return &Payload{Blocks: []Block{{Kind: kind, Text: text}}, Attrs: map[string]string{}}
}

func (p *Payload) Clone() *Payload {
	return &Payload{Blocks: slices.Clone(p.Blocks), Attrs: maps.Clone(p.Attrs)}
}

// Kind is the kind of the first block; empty payloads have no kind.
func (p *Payload) Kind() Kind {
	if len(p.Blocks) == 0 {
		return ""
	}
	return p.Blocks[0].Kind
}

func (p *Payload) Text() string {
	texts := make([]string, len(p.Blocks))
	for i, b := range p.Blocks {
		texts[i] = b.Text
	}
	return strings.Join(texts, "\n")
}

func (p *Payload) Tokens() int {
	n := 0
	for _, b := range p.Blocks {
		n += tokens.Estimate(b.Text)
	}
	return n
}

// SameContent reports whether blocks are identical; attrs are ignored.
func (p *Payload) SameContent(o *Payload) bool {
	return slices.Equal(p.Blocks, o.Blocks)
}

func (p *Payload) Set(k, v string) {
	if p.Attrs == nil {
		p.Attrs = map[string]string{}
	}
	p.Attrs[k] = v
}

// Rewrite replaces the text of every block whose Type is one of types with
// fn's result. Optimizers use it to touch only content they understand.
func (p *Payload) Rewrite(fn func(b Block) (string, error), types ...string) error {
	for i, b := range p.Blocks {
		if !slices.Contains(types, b.Type) {
			continue
		}
		text, err := fn(b)
		if err != nil {
			return err
		}
		p.Blocks[i].Text = text
	}
	return nil
}

// Key identifies the source of a payload across runs, so the ledger can
// remember which sources optimize badly. Empty means untrackable.
func (p *Payload) Key() string {
	if f := p.Attrs[AttrFile]; f != "" {
		return "file:" + f
	}
	if u := p.Attrs[AttrURL]; u != "" {
		return "url:" + u
	}
	if c := strings.Join(strings.Fields(p.Attrs[AttrCmd]), " "); c != "" {
		return "cmd:" + c
	}
	if t := p.Attrs[AttrTool]; t != "" {
		return "tool:" + t
	}
	return ""
}
