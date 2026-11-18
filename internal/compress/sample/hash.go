package sample

import (
	"encoding/json"
	"hash/maphash"
	"slices"
)

var seed = maphash.MakeSeed()

// hash fingerprints a decoded JSON value; equal values hash equally.
func hash(value any) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	writeHash(&h, value)
	return h.Sum64()
}

func writeHash(hasher *maphash.Hash, value any) {
	switch v := value.(type) {
	case nil:
		hasher.WriteByte('n')
	case bool:
		if v {
			hasher.WriteByte('t')
		} else {
			hasher.WriteByte('f')
		}
	case string:
		hasher.WriteByte('s')
		hasher.WriteString(v)
		hasher.WriteByte(0)
	case json.Number:
		hasher.WriteByte('#')
		hasher.WriteString(string(v))
		hasher.WriteByte(0)
	case []any:
		hasher.WriteByte('[')
		for _, x := range v {
			writeHash(hasher, x)
		}
		hasher.WriteByte(']')
	case map[string]any:
		hasher.WriteByte('{')
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			hasher.WriteString(k)
			hasher.WriteByte(0)
			writeHash(hasher, v[k])
		}
		hasher.WriteByte('}')
	}
}
