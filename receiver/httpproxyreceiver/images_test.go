package httpproxyreceiver

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngOf(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestShrinkImage(t *testing.T) {
	big := pngOf(t, 400, 200)
	out, ok := shrinkImage(big, "image/png", 20000)
	if !ok {
		t.Fatal("big image not shrunk")
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 200 || cfg.Height != 100 {
		t.Fatalf("got %dx%d (%v), want 200x100", cfg.Width, cfg.Height, err)
	}
	if again, _ := shrinkImage(big, "image/png", 20000); again != out {
		t.Fatal("not deterministic")
	}
	small := pngOf(t, 50, 50)
	if out, ok := shrinkImage(small, "image/png", 20000); ok || out != small {
		t.Fatal("small image changed")
	}
	if out, ok := shrinkImage("not base64!", "image/png", 10); ok || out != "not base64!" {
		t.Fatal("bad data changed")
	}
}

func TestShrinkImagesInResult(t *testing.T) {
	r, _, _, _, _ := setup(t)
	r.cfg.MaxImagePixels = 20000
	big := pngOf(t, 400, 200)
	b := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": big}}
	if !r.shrinkImages(t.Context(), b) || b["source"].(map[string]any)["data"] == big {
		t.Fatal("image block not shrunk")
	}
	r.cfg.MaxImagePixels = 0
	b2 := map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": big}}
	if r.shrinkImages(t.Context(), b2) {
		t.Fatal("shrunk with max_image_pixels 0")
	}
}
