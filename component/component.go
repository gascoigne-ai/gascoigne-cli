// Package component defines the contracts every pluggable part implements.
// Flow: receiver → filter → router → optimizer → exporter; extensions serve the rest.
package component

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/lukaszb/gascoigne/payload"
)

type Kind string

const (
	KindReceiver  Kind = "receiver"
	KindFilter    Kind = "filter"
	KindOptimizer Kind = "optimizer"
	KindExporter  Kind = "exporter"
	KindExtension Kind = "extension"
)

// ID is "type" or "type/name", so one type can be configured several times.
type ID struct {
	Type, Name string
}

func ParseID(s string) (ID, error) {
	typ, name, _ := strings.Cut(s, "/")
	if typ == "" || strings.Contains(name, "/") {
		return ID{}, fmt.Errorf("invalid component id %q", s)
	}
	return ID{typ, name}, nil
}

func (id ID) String() string {
	if id.Name == "" {
		return id.Type
	}
	return id.Type + "/" + id.Name
}

type Host interface {
	Extension(id ID) (Component, bool)
}

type Component interface {
	Start(ctx context.Context, host Host) error
	Shutdown(ctx context.Context) error
}

// Pipeline is what receivers feed. Process runs the full flow and returns the
// payload as exported, so request/response receivers can answer callers.
type Pipeline interface {
	Process(ctx context.Context, p *payload.Payload) (*payload.Payload, error)
}

type Receiver interface {
	Component
	// Run receives until input ends or ctx is done.
	Run(ctx context.Context) error
}

type Decision int

const (
	Continue Decision = iota
	Bypass            // skip optimization, export input verbatim
)

type Filter interface {
	Component
	Filter(ctx context.Context, p *payload.Payload) (Decision, error)
}

// Optimizer rewrites p in place. It should be deterministic: same input, same output.
type Optimizer interface {
	Component
	Optimize(ctx context.Context, p *payload.Payload) error
}

type Exporter interface {
	Component
	Export(ctx context.Context, p *payload.Payload) error
}

type Settings struct {
	ID       ID
	Logger   *slog.Logger
	Pipeline Pipeline // set for receivers only
}

// Nop gives components no-op lifecycle methods.
type Nop struct{}

func (Nop) Start(context.Context, Host) error { return nil }
func (Nop) Shutdown(context.Context) error    { return nil }
