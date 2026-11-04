package externaloptimizer

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
)

// Version is the wire contract version (docs/external-optimizer.md).
const Version = 1

// Block is the part of a payload block a child sees.
type Block struct {
	Kind string `json:"kind"`
	Type string `json:"type"`
	Text string `json:"text"`
}

// Request asks a child to shrink one block.
type Request struct {
	Version int               `json:"version"`
	ID      int               `json:"id,omitempty"` // rpc only
	Block   Block             `json:"block"`
	Attrs   map[string]string `json:"attrs"`
}

// Response is the child's answer. Text is a pointer so a missing field is an
// error instead of an empty rewrite.
type Response struct {
	Version    int     `json:"version"`
	ID         int     `json:"id,omitempty"`
	Text       *string `json:"text"`
	SkipReason string  `json:"skip_reason,omitempty"`
	Error      string  `json:"error,omitempty"`
}

// codec speaks one protocol to the child.
type codec interface {
	call(ctx context.Context, req Request) (Response, error)
	close()
}

type newCodec func(cfg *Config, log *slog.Logger) codec

// codecs is the protocol table; adding a protocol is one row.
var codecs = map[string]newCodec{
	Text: newTextCodec,
	JSON: newJSONCodec,
	RPC:  newRPCCodec,
}

func protocolNames() []string { return slices.Sorted(maps.Keys(codecs)) }

// check turns a decoded response into text or the child's error.
func (r Response) check() (string, error) {
	switch {
	case r.Error != "":
		return "", fmt.Errorf("child error: %s", r.Error)
	case r.Version != Version:
		return "", fmt.Errorf("child answered version %d, want %d", r.Version, Version)
	case r.Text == nil:
		return "", fmt.Errorf("child response has no text")
	}
	return *r.Text, nil
}

// attrEnv renders attrs as GASCOIGNE_ATTR_<KEY>=value, keys upper-cased and
// non-alphanumerics replaced by _, sorted so the environment is stable.
func attrEnv(attrs map[string]string) []string {
	env := make([]string, 0, len(attrs))
	for k, v := range attrs {
		key := strings.Map(func(char rune) rune {
			if char >= 'a' && char <= 'z' {
				return char - 32
			}
			if char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
				return char
			}
			return '_'
		}, k)
		env = append(env, "GASCOIGNE_ATTR_"+key+"="+v)
	}
	slices.Sort(env)
	return env
}
