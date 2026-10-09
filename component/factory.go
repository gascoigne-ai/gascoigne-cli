package component

import "context"

type Factory interface {
	Kind() Kind
	Type() string
	// DefaultConfig returns a pointer to a config struct; YAML decodes over it.
	DefaultConfig() any
	Create(ctx context.Context, set Settings, cfg any) (Component, error)
	// Family groups related optimizers (go, js, vcs, data...) in stats; may be empty.
	Family() string
}

// Validator is optionally implemented by component configs.
type Validator interface {
	Validate() error
}

type factory[C any] struct {
	kind   Kind
	typ    string
	family string
	def    func() *C
	create func(context.Context, Settings, *C) (Component, error)
}

type FactoryOption func(*factoryOptions)

type factoryOptions struct{ family string }

// WithFamily puts the component in a family, e.g. "js" for npm, bun and nx.
func WithFamily(family string) FactoryOption {
	return func(o *factoryOptions) { o.family = family }
}

// NewFactory builds a Factory with a typed config.
func NewFactory[C any](kind Kind, typ string, def func() *C, create func(context.Context, Settings, *C) (Component, error), opts ...FactoryOption) Factory {
	var o factoryOptions
	for _, opt := range opts {
		opt(&o)
	}
	return factory[C]{kind, typ, o.family, def, create}
}

func (f factory[C]) Kind() Kind         { return f.kind }
func (f factory[C]) Type() string       { return f.typ }
func (f factory[C]) DefaultConfig() any { return f.def() }
func (f factory[C]) Family() string     { return f.family }

func (f factory[C]) Create(ctx context.Context, set Settings, cfg any) (Component, error) {
	return f.create(ctx, set, cfg.(*C))
}
