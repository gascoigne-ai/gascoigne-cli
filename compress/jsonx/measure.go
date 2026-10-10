package jsonx

import "encoding/json"

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
