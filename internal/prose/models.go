package prose

import (
	"fmt"
	"maps"
	"math"
	"path/filepath"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/internal/prose/ortrt"
)

// The models Install can fetch, named as on Hugging Face. Kompress is a
// general prose compressor; Ctxprune was trained on what agents read (tool
// output, logs, code) as well as prose, and keeps fewer words at the same
// threshold on prose.
const (
	Kompress = "kompress-v2-base"
	Ctxprune = "ctxprune-small"
)

type file struct{ remote, local, sha256 string }

// spec is one model: where its files come from at a pinned revision, where
// they go, and which graph output holds the keep scores.
type spec struct {
	repo, revision string
	sub            string // directory under the model root; "" is the root itself
	files          []file // a download that does not match its hash is refused
	out            ortrt.Output
}

var models = map[string]spec{
	Kompress: {
		repo:     "chopratejas/kompress-v2-base",
		revision: "e00cc5e60ff73b01674ef8fa6da6c5acf6d54181",
		sub:      Kompress,
		files: []file{
			{"tokenizer.json", "tokenizer.json", "6c8aaa9a542084f2457eab775d4eeb51f92a70c0fd9de28d5edb0ddec3c08d30"},
			{"onnx/kompress-int8-wo.onnx", "model.onnx", "c47ddd764dec5cf91fd1f10cf69e5538dd2a49d913828da7f6a993931cc7904f"},
		},
		out: ortrt.Output{Name: "final_scores", Width: 1},
	},
	Ctxprune: {
		repo:     "darioooooo0o/ctxprune-small",
		revision: "c47bfc26508f437d9d737550e19a2541e3049f83",
		sub:      Ctxprune,
		files: []file{
			{"tokenizer.json", "tokenizer.json", "609d8f4c067cd3950f88594c5a802616cea245823836ef5848ee4fc40aab5b6f"},
			{"onnx/model.onnx", "model.onnx", "fe605d35ebe4219c4adff23b6ae6ce139993a67148e4ba78f60cd677c4f20036"},
		},
		out: ortrt.Output{Name: "logits", Width: 2},
	},
}

// Names lists the models, sorted.
func Names() []string { return slices.Sorted(maps.Keys(models)) }

// lookup finds a model and its directory under root.
func lookup(root, name string) (spec, string, error) {
	s, ok := models[name]
	if !ok {
		return spec{}, "", fmt.Errorf("unknown prose model %q (have %v)", name, Names())
	}
	return s, filepath.Join(root, s.sub), nil
}

// keepProb turns logits pairs (drop, keep) into the probability of keep.
func keepProb(logits []float32) []float32 {
	p := make([]float32, len(logits)/2)
	for i := range p {
		p[i] = float32(1 / (1 + math.Exp(float64(logits[2*i]-logits[2*i+1]))))
	}
	return p
}
