// Package service builds components from config, wires pipelines and runs them.
package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/internal/config"
)

type Options struct {
	Logger *slog.Logger
	// Attrs are added to every payload unless already set (e.g. CLI -attr flags).
	Attrs map[string]string
}

type Service struct {
	log        *slog.Logger
	attrs      map[string]string
	factories  map[component.Kind]map[string]component.Factory
	extensions map[component.ID]component.Component
	filters    map[string]component.Filter
	optimizers map[string]component.Optimizer
	exporters  map[string]component.Exporter
	receivers  map[string][]component.Receiver // by pipeline
	pipelines  map[string]*Pipeline
	ledger     component.Ledger
	started    []component.Component
}

func New(ctx context.Context, cfg *config.Config, factories []component.Factory, opts Options) (*Service, error) {
	s := &Service{
		log:        opts.Logger,
		attrs:      opts.Attrs,
		factories:  map[component.Kind]map[string]component.Factory{},
		extensions: map[component.ID]component.Component{},
		filters:    map[string]component.Filter{},
		optimizers: map[string]component.Optimizer{},
		exporters:  map[string]component.Exporter{},
		pipelines:  map[string]*Pipeline{},
		receivers:  map[string][]component.Receiver{},
	}
	if s.log == nil {
		s.log = slog.New(slog.DiscardHandler)
	}
	for _, f := range factories {
		if s.factories[f.Kind()] == nil {
			s.factories[f.Kind()] = map[string]component.Factory{}
		}
		s.factories[f.Kind()][f.Type()] = f
	}
	if err := s.build(ctx, cfg); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) build(ctx context.Context, cfg *config.Config) error {
	for _, id := range slices.Sorted(maps.Keys(cfg.Extensions)) {
		c, err := s.create(ctx, component.KindExtension, id, cfg.Extensions[id], nil)
		if err != nil {
			return err
		}
		pid, _ := component.ParseID(id)
		s.extensions[pid] = c
		if l, ok := c.(component.Ledger); ok && s.ledger == nil {
			s.ledger = l
		}
	}
	if err := buildAll(ctx, s, component.KindFilter, cfg.Filters, s.filters); err != nil {
		return err
	}
	if err := buildAll(ctx, s, component.KindOptimizer, cfg.Optimizers, s.optimizers); err != nil {
		return err
	}
	if err := buildAll(ctx, s, component.KindExporter, cfg.Exporters, s.exporters); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Pipelines)) {
		pc := cfg.Pipelines[name]
		pl := &Pipeline{name: name, svc: s, router: cfg.RouterFor(pc)}
		if len(pl.router.Optimizers()) > 0 && s.ledger == nil {
			return fmt.Errorf("pipeline %s: optimizers need a ledger extension to keep originals retrievable", name)
		}
		for _, id := range pc.Filters {
			pl.filters = append(pl.filters, named[component.Filter]{id, s.filters[id]})
		}
		for _, id := range pc.Exporters {
			pl.exporters = append(pl.exporters, named[component.Exporter]{id, s.exporters[id]})
		}
		for _, id := range pc.Receivers {
			c, err := s.create(ctx, component.KindReceiver, id, cfg.Receivers[id], pl)
			if err != nil {
				return err
			}
			r, ok := c.(component.Receiver)
			if !ok {
				return fmt.Errorf("receiver %s: does not implement Receiver", id)
			}
			s.receivers[name] = append(s.receivers[name], r)
		}
		s.pipelines[name] = pl
	}
	return nil
}

func buildAll[T any](ctx context.Context, s *Service, kind component.Kind, comps config.Components, into map[string]T) error {
	for _, id := range slices.Sorted(maps.Keys(comps)) {
		c, err := s.create(ctx, kind, id, comps[id], nil)
		if err != nil {
			return err
		}
		t, ok := c.(T)
		if !ok {
			return fmt.Errorf("%s %s: does not implement %s", kind, id, kind)
		}
		into[id] = t
	}
	return nil
}

func (s *Service) create(ctx context.Context, kind component.Kind, rawID string, node config.Node, pl *Pipeline) (component.Component, error) {
	id, err := component.ParseID(rawID)
	if err != nil {
		return nil, err
	}
	f, ok := s.factories[kind][id.Type]
	if !ok {
		return nil, fmt.Errorf("%s %s: unknown type %q", kind, id, id.Type)
	}
	cfg := f.DefaultConfig()
	if err := config.DecodeNode(node, cfg); err != nil {
		return nil, fmt.Errorf("%s %s: %w", kind, id, err)
	}
	if v, ok := cfg.(component.Validator); ok {
		if err := v.Validate(); err != nil {
			return nil, fmt.Errorf("%s %s: %w", kind, id, err)
		}
	}
	set := component.Settings{ID: id, Logger: s.log.With(string(kind), id.String())}
	if pl != nil {
		set.Pipeline = pl
	}
	return f.Create(ctx, set, cfg)
}

// Start starts components dependencies-first: extensions, exporters,
// optimizers, filters, receivers. On failure, already started ones are shut down.
func (s *Service) Start(ctx context.Context) error {
	var order []component.Component
	for _, id := range slices.SortedFunc(maps.Keys(s.extensions), func(a, b component.ID) int {
		return cmp.Compare(a.String(), b.String())
	}) {
		order = append(order, s.extensions[id])
	}
	order = appendSorted(order, s.exporters)
	order = appendSorted(order, s.optimizers)
	order = appendSorted(order, s.filters)
	for _, name := range s.PipelineNames() {
		for _, r := range s.receivers[name] {
			order = append(order, r)
		}
	}
	for _, c := range order {
		if err := c.Start(ctx, s); err != nil {
			return errors.Join(err, s.Shutdown(ctx))
		}
		s.started = append(s.started, c)
	}
	return nil
}

func appendSorted[T component.Component](order []component.Component, m map[string]T) []component.Component {
	for _, id := range slices.Sorted(maps.Keys(m)) {
		order = append(order, m[id])
	}
	return order
}

// Run runs the receivers of the named pipelines (all when none are named)
// until they finish or ctx is done.
func (s *Service) Run(ctx context.Context, pipelines ...string) error {
	if len(pipelines) == 0 {
		pipelines = s.PipelineNames()
	}
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for _, name := range pipelines {
		if _, ok := s.pipelines[name]; !ok {
			return fmt.Errorf("no pipeline %q", name)
		}
		for _, r := range s.receivers[name] {
			wg.Go(func() {
				if err := r.Run(ctx); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
				}
			})
		}
	}
	wg.Wait()
	return errors.Join(errs...)
}

// Shutdown stops started components in reverse order.
func (s *Service) Shutdown(ctx context.Context) error {
	var errs []error
	for _, c := range slices.Backward(s.started) {
		errs = append(errs, c.Shutdown(ctx))
	}
	s.started = nil
	return errors.Join(errs...)
}

func (s *Service) Extension(id component.ID) (component.Component, bool) {
	c, ok := s.extensions[id]
	return c, ok
}

// Ledger returns the ledger extension, or nil when none is configured.
func (s *Service) Ledger() component.Ledger { return s.ledger }

func (s *Service) Pipeline(name string) (*Pipeline, bool) {
	pl, ok := s.pipelines[name]
	return pl, ok
}

func (s *Service) PipelineNames() []string {
	return slices.Sorted(maps.Keys(s.pipelines))
}

type named[T any] struct {
	id string
	c  T
}
