package jsonoptimizer

import (
	"encoding/json"
	"hash/maphash"
	"slices"
)

// size estimates the compact JSON length of v without encoding it; escapes
// are ignored. Good enough to compare forms and count tokens.
func size(v any) int {
	switch v := v.(type) {
	case nil:
		return 4
	case bool:
		if v {
			return 4
		}
		return 5
	case string:
		return len(v) + 2
	case json.Number:
		return len(v)
	case []any:
		n := 2 + max(len(v)-1, 0)
		for _, x := range v {
			n += size(x)
		}
		return n
	case []string:
		n := 2 + max(len(v)-1, 0)
		for _, x := range v {
			n += len(x) + 2
		}
		return n
	case map[string]any:
		n := 2 + max(len(v)-1, 0)
		for k, x := range v {
			n += len(k) + 3 + size(x)
		}
		return n
	}
	return 8
}

var seed = maphash.MakeSeed()

// hash fingerprints a decoded JSON value; equal values hash equally.
func hash(v any) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	writeHash(&h, v)
	return h.Sum64()
}

func writeHash(h *maphash.Hash, v any) {
	switch v := v.(type) {
	case nil:
		h.WriteByte('n')
	case bool:
		if v {
			h.WriteByte('t')
		} else {
			h.WriteByte('f')
		}
	case string:
		h.WriteByte('s')
		h.WriteString(v)
		h.WriteByte(0)
	case json.Number:
		h.WriteByte('#')
		h.WriteString(string(v))
		h.WriteByte(0)
	case []any:
		h.WriteByte('[')
		for _, x := range v {
			writeHash(h, x)
		}
		h.WriteByte(']')
	case map[string]any:
		h.WriteByte('{')
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			h.WriteString(k)
			h.WriteByte(0)
			writeHash(h, v[k])
		}
		h.WriteByte('}')
	}
}
