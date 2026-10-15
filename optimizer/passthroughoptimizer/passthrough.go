// Package passthroughoptimizer leaves payloads untouched; a baseline and route placeholder.
package passthroughoptimizer

import (
	"context"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

type Optimizer struct{ component.Nop }

func (Optimizer) Optimize(context.Context, *payload.Payload) error { return nil }
