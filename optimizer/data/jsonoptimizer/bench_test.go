package jsonoptimizer

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func benchInputs(b *testing.B) map[string]string {
	b.Helper()
	in := map[string]string{}
	for name, path := range map[string]string{
		"github-repos": "../../../testdata/eval/json/github-repos.json",
		"package-lock": "../../../testdata/eval/json/package-lock.json",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		in[name] = string(data)
	}
	var events []map[string]any
	for i := range 20000 {
		ev := map[string]any{"i": i, "level": "info", "dur_ms": 10 + i%50, "path": fmt.Sprintf("/api/items/%d", i%300), "msg": "request handled"}
		if i%5000 == 4999 {
			ev["level"], ev["error"] = "error", "boom"
		}
		events = append(events, ev)
	}
	data, _ := json.Marshal(events)
	in["events-20k"] = string(data)
	return in
}

func BenchmarkOptimize(b *testing.B) {
	o := &Optimizer{cfg: defaultConfig()}
	for name, text := range benchInputs(b) {
		b.Run(name, func(b *testing.B) {
			b.SetBytes(int64(len(text)))
			for b.Loop() {
				p := &payload.Payload{Blocks: []payload.Block{{Text: text, Type: detect.JSON}}}
				if err := o.Optimize(b.Context(), p); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
