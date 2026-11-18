// Package component defines the contracts every pluggable part implements.
// Flow: receiver → filter → router → optimizer → exporter; extensions serve the rest.
package component

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// Kind names the role a component plays in a pipeline.
type Kind string

// The component kinds. The config file has one section per kind, and a Factory
// reports its kind so the service knows where to use what it builds.
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

// ParseID parses the "type" or "type/name" form of an ID. It rejects an empty type
// and a name that contains another slash.
func ParseID(text string) (ID, error) {
	typ, name, _ := strings.Cut(text, "/")
	if typ == "" || strings.Contains(name, "/") {
		return ID{}, fmt.Errorf("invalid component id %q", text)
	}
	return ID{typ, name}, nil
}

// String returns the form that ParseID reads back.
func (id ID) String() string {
	if id.Name == "" {
		return id.Type
	}
	return id.Type + "/" + id.Name
}

// Host is the service a started component can ask for the extensions it depends
// on, such as the ledger.
type Host interface {
	Extension(id ID) (Component, bool)
}

// Component is the lifecycle every pluggable part shares. The service starts
// components dependencies-first and shuts them down in reverse order.
type Component interface {
	Start(ctx context.Context, host Host) error
	Shutdown(ctx context.Context) error
}

// Pipeline is what receivers feed. Process runs the full flow and returns the
// payload as exported, so request/response receivers can answer callers.
type Pipeline interface {
	Process(ctx context.Context, data *payload.Payload) (*payload.Payload, error)
}

// Receiver brings payloads into a pipeline from outside: stdin, a command, an
// agent hook, an MCP client or an LLM API request.
type Receiver interface {
	Component
	// Run receives until input ends or ctx is done.
	Run(ctx context.Context) error
}

// Decision is what a Filter says should happen to a payload.
type Decision int

const (
	// Continue leaves the payload for the next filter and then the optimizers.
	Continue Decision = iota
	Bypass            // skip optimization, export input verbatim
)

// Filter inspects a payload before routing and either lets it continue or bypasses
// optimization. It may also tag the payload, with attrs or block types, for later stages.
type Filter interface {
	Component
	Filter(ctx context.Context, data *payload.Payload) (Decision, error)
}

// Optimizer rewrites p in place. It should be deterministic: same input, same output.
type Optimizer interface {
	Component
	Optimize(ctx context.Context, data *payload.Payload) error
}

// Exporter delivers the final payload, optimized or bypassed, to its destination.
type Exporter interface {
	Component
	Export(ctx context.Context, data *payload.Payload) error
}

// Settings is what the service hands a factory when it creates a component.
type Settings struct {
	ID       ID
	Logger   *slog.Logger
	Pipeline Pipeline // set for receivers only
}

// Nop gives components no-op lifecycle methods.
type Nop struct{}

// Start does nothing and succeeds.
func (Nop) Start(context.Context, Host) error { return nil }

// Shutdown does nothing and succeeds.
func (Nop) Shutdown(context.Context) error { return nil }
