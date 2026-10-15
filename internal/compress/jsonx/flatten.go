package jsonx

// Flatten copies m into dst with nested objects (and single-object lists)
// joined into dotted keys: {"spec": {"image": x}} becomes {"spec.image": x}.
// Arrays of wide, nested, mostly identical objects (API lists, manifests)
// then tabulate leaf by leaf, so a leaf equal in every item hoists into
// _same instead of repeating a whole subtree per row.
func Flatten(dst map[string]any, prefix string, m map[string]any) {
	for k, x := range m {
		key := prefix + k
		if l, ok := x.([]any); ok && len(l) == 1 {
			if one, ok := l[0].(map[string]any); ok && !isTable(one) {
				Flatten(dst, key+"[0].", one)
				continue
			}
		}
		if sub, ok := x.(map[string]any); ok && len(sub) > 0 && !isTable(sub) {
			Flatten(dst, key+".", sub)
			continue
		}
		dst[key] = x
	}
}

// isTable reports a map the crusher already turned into a table.
func isTable(m map[string]any) bool {
	_, ok := m["_cols"]
	return ok
}

// nested reports whether any object item holds a nested object.
func nested(items []any) bool {
	for _, it := range items {
		obj, ok := it.(map[string]any)
		if !ok {
			continue
		}
		for _, v := range obj {
			if sub, ok := v.(map[string]any); ok && len(sub) > 0 && !isTable(sub) {
				return true
			}
		}
	}
	return false
}
