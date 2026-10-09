// Package stdinreceiver reads all of stdin as one payload.
package stdinreceiver

import (
	"context"
	"io"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

type Receiver struct {
	component.Nop
	cfg  *Config
	next component.Pipeline
	in   io.Reader
}

func (r *Receiver) Run(ctx context.Context) error {
	data, err := io.ReadAll(r.in)
	if err != nil {
		return err
	}
	_, err = r.next.Process(ctx, payload.New(r.cfg.Kind, string(data)))
	return err
}
