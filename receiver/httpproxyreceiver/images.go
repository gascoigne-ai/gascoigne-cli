package httpproxyreceiver

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/component"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

// shrinkImage downscales a base64 PNG or JPEG above maxPixels to fit it
// (aspect kept, box filter: each output pixel averages the source pixels it
// covers). Image tokens scale with area, and screenshots are often larger
// than the model reads anyway. The result is deterministic, so later
// requests send the same bytes; on any problem the image stays.
func shrinkImage(data, mediaType string, maxPixels int) (string, bool) {
	out, _, _, ok := shrink(data, mediaType, maxPixels)
	return out, ok
}

// shrink is shrinkImage plus the pixel counts before and after.
func shrink(data, mediaType string, maxPixels int) (out string, before, after int, ok bool) {
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

// pixelsPerToken is how many pixels Anthropic bills as one image token.
const pixelsPerToken = 750

// keepImage saves the original image (base64) so `gascoigne raw` returns
// it, and records the saving in image tokens.
func (r *Receiver) keepImage(ctx context.Context, data, mediaType string, before, after int) {
	if r.ledger == nil {
		return
	}
	p := payload.New(payload.ToolResult, data)
	p.Set("mime", mediaType)
	key := "image:" + cacheKey(call{name: mediaType}, data)[:16]
	id, err := r.ledger.SaveRaw(ctx, key, p)
	if err != nil {
		r.log.Warn("save original image", "err", err)
		return
	}
	in, out := before/pixelsPerToken, after/pixelsPerToken
	err = r.ledger.Record(ctx, component.Event{
		TS: time.Now(), Pipeline: "proxy", Key: key, Route: "image", Command: "image " + mediaType,
		Optimizers: []string{"image"}, RawTokens: in, OutTokens: out, RawID: id, Outcome: component.Optimized,
		Steps: []component.Step{{Optimizer: "image", Family: "core", RawTokens: in, OutTokens: out}},
	})
	if err != nil {
		r.log.Warn("record image", "err", err)
	}
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
