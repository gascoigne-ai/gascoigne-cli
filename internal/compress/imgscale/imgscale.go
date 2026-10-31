// Package imgscale downscales base64 PNG and JPEG images. The LLM proxy and
// the Read hook share it, so a screenshot shrinks the same way in both.
package imgscale

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// PixelsPerToken is how many pixels Anthropic bills as one image token.
const PixelsPerToken = 750

// Shrink downscales a base64 PNG or JPEG above maxPixels to fit it (aspect
// kept, box filter: each output pixel averages the source pixels it covers).
// Image tokens scale with area, and screenshots are often larger than the
// model reads anyway. The result is deterministic, so later requests send
// the same bytes. It returns the pixel counts before and after; on any
// problem (other format, small image, bad data) data stays and ok is false.
func Shrink(data, mediaType string, maxPixels int) (out string, before, after int, ok bool) {
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return data, 0, 0, false
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width*cfg.Height <= maxPixels || (format != "png" && format != "jpeg") {
		return data, 0, 0, false
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return data, 0, 0, false
	}
	scale := math.Sqrt(float64(maxPixels) / float64(cfg.Width*cfg.Height))
	w, h := max(1, int(float64(cfg.Width)*scale)), max(1, int(float64(cfg.Height)*scale))
	dst := boxScale(src, w, h)
	var buf bytes.Buffer
	if mediaType == "image/jpeg" || format == "jpeg" {
		err = jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 85})
	} else {
		err = png.Encode(&buf, dst)
	}
	if err != nil {
		return data, 0, 0, false
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), cfg.Width * cfg.Height, w * h, true
}

// Keep saves the original image (base64) so `gascoigne raw` returns it, and
// records the saving in image tokens. ev holds who and where (TS, Session,
// Agent, Model, Pipeline); the rest is filled in.
func Keep(ctx context.Context, l component.Ledger, ev component.Event, data, mediaType string, before, after int) error {
	p := payload.New(payload.ToolResult, data)
	p.Set("mime", mediaType)
	sum := sha256.Sum256([]byte(mediaType + "\x00" + data))
	ev.Key = "image:" + hex.EncodeToString(sum[:8])
	id, err := l.SaveRaw(ctx, ev.Key, p)
	if err != nil {
		return fmt.Errorf("save original image: %w", err)
	}
	in, out := before/PixelsPerToken, after/PixelsPerToken
	ev.Route, ev.Command, ev.Optimizers = "image", "image "+mediaType, []string{"image"}
	ev.RawTokens, ev.OutTokens, ev.RawID, ev.Outcome = in, out, id, component.Optimized
	ev.Steps = []component.Step{{Optimizer: "image", Family: "core", RawTokens: in, OutTokens: out}}
	if err := l.Record(ctx, ev); err != nil {
		return fmt.Errorf("record image: %w", err)
	}
	return nil
}

func boxScale(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+max((y+1)*b.Dy()/h, y*b.Dy()/h+1)
		for x := range w {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+max((x+1)*b.Dx()/w, x*b.Dx()/w+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					c := color.NRGBAModel.Convert(src.At(sx, sy)).(color.NRGBA)
					r, g, bl, a, n = r+uint64(c.R), g+uint64(c.G), bl+uint64(c.B), a+uint64(c.A), n+1
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(bl / n), uint8(a / n)})
		}
	}
	return dst
}
