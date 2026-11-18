package httpproxyreceiver

import "testing"

func TestReplaceContentBlocks(t *testing.T) {
	r, _, _, _, _ := setup(t)
	r.cfg.MaxImagePixels = 20000
	toolCall := call{name: "Read", input: map[string]any{"file_path": "a.go"}}
	text := "// note\ncode\n"
	img := pngOf(t, 400, 200)
	image := func() map[string]any {
		return map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": img}}
	}

	t.Run("string", func(t *testing.T) {
		got, changed := replaceContent(t.Context(), r, toolCall, text, meta{})
		if !changed || got == text {
			t.Fatalf("string not shortened: %v %q", changed, got)
		}
	})
	t.Run("text blocks of each type", func(t *testing.T) {
		for _, typ := range []string{"text", "input_text", "output_text"} {
			block := map[string]any{"type": typ, "text": text}
			_, changed := replaceContent(t.Context(), r, toolCall, []any{block}, meta{})
			if !changed || block["text"] == text {
				t.Errorf("%s block not shortened", typ)
			}
		}
	})
	t.Run("other blocks and unchanged text pass", func(t *testing.T) {
		blocks := []any{
			map[string]any{"type": "tool_reference", "text": text},
			map[string]any{"type": "text"},
			map[string]any{"type": "text", "text": "plain\n"},
		}
		if _, changed := replaceContent(t.Context(), r, toolCall, blocks, meta{}); changed {
			t.Fatal("changed blocks that need no change")
		}
		if blocks[0].(map[string]any)["text"] != text {
			t.Fatal("block of another type was rewritten")
		}
	})
	t.Run("image and text together", func(t *testing.T) {
		pic, block := image(), map[string]any{"type": "text", "text": text}
		_, changed := replaceContent(t.Context(), r, toolCall, []any{pic, block}, meta{})
		if !changed || pic["source"].(map[string]any)["data"] == img || block["text"] == text {
			t.Fatal("image or text left as is")
		}
	})
	t.Run("other content", func(t *testing.T) {
		if _, changed := replaceContent(t.Context(), r, toolCall, 42, meta{}); changed {
			t.Fatal("changed a number")
		}
	})
}
