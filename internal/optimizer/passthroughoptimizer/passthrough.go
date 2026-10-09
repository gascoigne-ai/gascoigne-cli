// Package passthroughoptimizer leaves payloads untouched; a baseline and route placeholder.
package passthroughoptimizer

import (
	"context"

	"github.com/lukaszb/gascoigne/internal/component"
	"github.com/lukaszb/gascoigne/internal/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(context.Context, *payload.Payload) error { return nil }
