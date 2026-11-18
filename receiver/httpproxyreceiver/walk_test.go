package httpproxyreceiver

import (
	"slices"
	"testing"
)

func TestObjectsYieldsMapElementsOnly(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{"list of maps", []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}}, []string{"a", "b"}},
		{"skips non maps", []any{"x", 3, map[string]any{"id": "a"}, nil}, []string{"a"}},
		{"string is not a list", "text", nil},
		{"nil", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []string
			for object := range objects(tc.value) {
				got = append(got, object["id"].(string))
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMessagesWalksEachKey(t *testing.T) {
	doc := map[string]any{
		"messages": []any{map[string]any{"id": "m1"}},
		"input":    []any{map[string]any{"id": "i1"}},
	}
	var got []string
	for message := range messages(doc, "messages", "input") {
		got = append(got, message["id"].(string))
	}
	if !slices.Equal(got, []string{"m1", "i1"}) {
		t.Errorf("got %v", got)
	}
}
