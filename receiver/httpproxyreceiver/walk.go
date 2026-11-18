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
