package imgscale

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"
)

// pngOf is a w×h gradient PNG, base64 encoded.
func pngOf(t testing.TB, w, h int) string {
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

func TestShrink(t *testing.T) {
	big := pngOf(t, 400, 200)
	out, before, after, ok := Shrink(big, "image/png", 20000)
	if !ok || before != 80000 || after != 20000 {
		t.Fatalf("big image: ok=%v before=%d after=%d", ok, before, after)
	}
	raw, _ := base64.StdEncoding.DecodeString(out)
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 200 || cfg.Height != 100 {
		t.Fatalf("got %dx%d (%v), want 200x100", cfg.Width, cfg.Height, err)
	}
	if again, _, _, _ := Shrink(big, "image/png", 20000); again != out {
		t.Fatal("not deterministic")
	}
	small := pngOf(t, 50, 50)
	if out, _, _, ok := Shrink(small, "image/png", 20000); ok || out != small {
		t.Fatal("small image changed")
	}
	if out, _, _, ok := Shrink("not base64!", "image/png", 10); ok || out != "not base64!" {
		t.Fatal("bad data changed")
	}
}
