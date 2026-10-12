package httpproxyreceiver

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func shaped(cfg Config, system any, firstMsg string) (map[string]any, bool) {
	doc := map[string]any{"messages": []any{map[string]any{"role": "user", "content": firstMsg}}}
	if system != nil {
		doc["system"] = system
	}
	r := &Receiver{cfg: &cfg}
	return doc, r.shapeSystem(doc)
}

func TestShapeSystem(t *testing.T) {
	on := Config{ShapeOutput: true}
	cached := map[string]any{"type": "text", "text": "You are an agent.", "cache_control": map[string]any{"type": "ephemeral"}}
	tests := []struct {
		name    string
		cfg     Config
		system  any
		changed bool
		want    any
	}{
		{"off by default", Config{}, "You are an agent.", false, "You are an agent."},
		{"string", on, "You are an agent.\n", true, "You are an agent.\n\n" + shapeNote},
		{"none", on, nil, true, shapeNote},
		{"blocks: appended after the cached one", on, []any{cached}, true,
			[]any{cached, map[string]any{"type": "text", "text": shapeNote}}},
		{"empty list", on, []any{}, true, []any{map[string]any{"type": "text", "text": shapeNote}}},
		{"already there (string)", on, "x\n\n" + shapeNote, false, "x\n\n" + shapeNote},
		{"already there (blocks)", on, []any{map[string]any{"type": "text", "text": shapeNote}}, false,
			[]any{map[string]any{"type": "text", "text": shapeNote}}},
		{"unknown shape", on, 42, false, 42},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, changed := shaped(tt.cfg, tt.system, "hi")
			if changed != tt.changed || !reflect.DeepEqual(doc["system"], tt.want) {
				t.Fatalf("changed = %v, system = %#v\nwant %v, %#v", changed, doc["system"], tt.changed, tt.want)
			}
		})
	}
}

func TestShapeKeepsTheCachedPrefix(t *testing.T) {
	cached := map[string]any{"type": "text", "text": "You are an agent.", "cache_control": map[string]any{"type": "ephemeral"}}
	before, _ := json.Marshal(cached)
	doc, _ := shaped(Config{ShapeOutput: true}, []any{cached}, "hi")
	first, _ := json.Marshal(doc["system"].([]any)[0])
	if string(first) != string(before) {
		t.Errorf("the cached block changed:\n%s\n%s", before, first)
	}
	again, _ := shaped(Config{ShapeOutput: true}, []any{cached}, "hi")
	a, _ := json.Marshal(doc)
	b, _ := json.Marshal(again)
	if string(a) != string(b) {
		t.Error("two requests of one conversation must carry the same bytes")
	}
}

func TestShapeHoldout(t *testing.T) {
	cfg := Config{ShapeOutput: true, ShapeHoldout: 0.3}
	held, total := 0, 2000
	for i := range total {
		msg := fmt.Sprintf("conversation %d", i)
		_, changed := shaped(cfg, "sys", msg)
		_, again := shaped(cfg, "sys", msg)
		if changed != again {
			t.Fatalf("the arm of %q flipped between requests", msg)
		}
		if !changed {
			held++
		}
	}
	if share := float64(held) / float64(total); share < 0.25 || share > 0.35 {
		t.Errorf("held out %.2f of conversations, want about 0.30", share)
	}
	if _, changed := shaped(Config{ShapeOutput: true, ShapeHoldout: 1}, "sys", "x"); changed {
		t.Error("holdout 1 shapes nothing")
	}
	if _, changed := shaped(Config{ShapeOutput: true, ShapeHoldout: 0}, "sys", strings.Repeat("y", 10)); !changed {
		t.Error("holdout 0 shapes everything")
	}
}

func TestShapeHoldoutValidated(t *testing.T) {
	c := Config{Anthropic: "http://a", OpenAI: "http://o", Gemini: "http://g", Bedrock: "http://b", CacheEntries: 100, ShapeHoldout: 1.5}
	if c.Validate() == nil {
		t.Error("a share above 1 must be refused")
	}
}

func TestShapeThroughTheProxy(t *testing.T) {
	r, _, proxy, got, _ := setup(t)
	body := anthropicBody("plain output", "Bash")
	post(t, proxy.URL, body, nil)
	var doc map[string]any
	json.Unmarshal(*got, &doc)
	if doc["system"] != "keep me" {
		t.Fatalf("off by default, system = %v", doc["system"])
	}
	r.cfg.ShapeOutput = true
	post(t, proxy.URL, body, nil)
	json.Unmarshal(*got, &doc)
	if doc["system"] != "keep me\n\n"+shapeNote {
		t.Fatalf("system = %q", doc["system"])
	}
	if doc["max_tokens"] != float64(64000) {
		t.Errorf("rest of the body changed: %v", doc)
	}
}
