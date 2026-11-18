package httpproxyreceiver

import "iter"

// objects yields the JSON objects of a decoded list; any other value, and
// list elements that are not objects, are skipped.
func objects(value any) iter.Seq[map[string]any] {
	return func(yield func(map[string]any) bool) {
		list, _ := value.([]any)
		for _, element := range list {
			if object, ok := element.(map[string]any); ok && !yield(object) {
				return
			}
		}
	}
}

// messages yields every message of the request body's lists under keys, in
// key order: "messages" (Anthropic, chat), "input" (responses), "contents".
func messages(doc map[string]any, keys ...string) iter.Seq[map[string]any] {
	return func(yield func(map[string]any) bool) {
		for _, key := range keys {
			for message := range objects(doc[key]) {
				if !yield(message) {
					return
				}
			}
		}
	}
}

func asList(value any) []any {
	list, _ := value.([]any)
	return list
}

// str is the string at key of a decoded object; "" when it is absent or not a string.
func str(object map[string]any, key string) string {
	s, _ := object[key].(string)
	return s
}

// obj is the object at key of a decoded object; nil when it is absent or not an object.
// Unknown fields stay in the map, so a body forwarded after an edit keeps them.
func obj(object map[string]any, key string) map[string]any {
	o, _ := object[key].(map[string]any)
	return o
}
