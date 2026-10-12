package httpproxyreceiver

import (
	"encoding/json"
	"hash/fnv"
	"strings"
)

// shapeNote is what shape_output adds to the end of the system prompt. Output
// tokens cost several times input tokens, and much of what a model writes
// back is narration of steps and restated context.
const shapeNote = "Be terse: do not restate the question or narrate steps; report results and decisions."

// shapeSystem ends the system prompt of an Anthropic request with shapeNote.
// The note goes after every block already there, so the cached part of the
// prompt keeps its bytes, and it is the same bytes in every request of a
// conversation. A conversation in the holdout share gets no note. It reports
// whether the request changed.
func (r *Receiver) shapeSystem(doc map[string]any) bool {
	if !r.cfg.ShapeOutput || heldOut(doc, r.cfg.ShapeHoldout) {
		return false
	}
	switch s := doc["system"].(type) {
	case nil:
		doc["system"] = shapeNote
	case string:
		if strings.HasSuffix(s, shapeNote) {
			return false
		}
		doc["system"] = strings.TrimRight(s, "\n") + "\n\n" + shapeNote
	case []any:
		if len(s) > 0 {
			if last, _ := s[len(s)-1].(map[string]any); last["text"] == shapeNote {
				return false
			}
		}
		doc["system"] = append(s, map[string]any{"type": "text", "text": shapeNote})
	default:
		return false
	}
	return true
}

// heldOut decides from the conversation's first message, which later
// requests of the conversation repeat, so the arm never flips mid-way (that
// would change the prompt and spend the cache).
func heldOut(doc map[string]any, share float64) bool {
	if share <= 0 {
		return false
	}
	var first any
	if msgs, _ := doc["messages"].([]any); len(msgs) > 0 {
		first = msgs[0]
	}
	b, _ := json.Marshal(first) // map keys marshal sorted: stable
	h := fnv.New32a()
	h.Write(b)
	return float64(h.Sum32()%10000)/10000 < share
}
