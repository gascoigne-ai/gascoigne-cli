package component

import "context"

type Factory interface {
	Kind() Kind
	Type() string
	// DefaultConfig returns a pointer to a config struct; YAML decodes over it.
	DefaultConfig() any
	Create(ctx context.Context, set Settings, cfg any) (Component, error)
}

// Validator is optionally implemented by component configs.
type Validator interface {
	Validate() error
}

type factory[C any] struct {
	kind   Kind
	typ    string
	def    func() *C
	create func(context.Context, Settings, *C) (Component, error)
}

// NewFactory builds a Factory with a typed config.
func NewFactory[C any](kind Kind, typ string, def func() *C, create func(context.Context, Settings, *C) (Component, error)) Factory {
	return factory[C]{kind, typ, def, create}
}

func (f factory[C]) Kind() Kind         { return f.kind }
func (f factory[C]) Type() string       { return f.typ }
func (f factory[C]) DefaultConfig() any { return f.def() }

func (f factory[C]) Create(ctx context.Context, set Settings, cfg any) (Component, error) {
	return f.create(ctx, set, cfg.(*C))
}
