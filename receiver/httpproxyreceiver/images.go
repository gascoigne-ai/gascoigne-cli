package httpproxyreceiver

import (
	"context"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/imgscale"
)

// keepImage saves the original image so `gascoigne raw` returns it, and
// records the saving in image tokens.
func (r *Receiver) keepImage(ctx context.Context, data, mediaType string, before, after int) {
	if r.ledger == nil {
		return
	}
	ev := component.Event{TS: time.Now(), Pipeline: "proxy"}
	if err := imgscale.Keep(ctx, r.ledger, ev, data, mediaType, before, after); err != nil {
		r.log.Warn("keep image", "err", err)
	}
}
