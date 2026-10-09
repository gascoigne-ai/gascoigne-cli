// Package passthroughoptimizer leaves payloads untouched; a baseline and route placeholder.
package passthroughoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/component"
	"github.com/lukaszb/gascoigne/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(context.Context, *payload.Payload) error { return nil }
