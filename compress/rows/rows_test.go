package rows

import (
	"encoding/json"
	"slices"
	"strconv"
	"testing"
)

func TestRelevance(t *testing.T) {
	docs := []any{
		map[string]any{"title": "install guide"},
		map[string]any{"title": "checkout flow fails on retry"},
		map[string]any{"title": "checkout"},
		map[string]any{"title": "unrelated"},
	}
	got := relevance(docs, []int{0, 1, 2, 3}, Terms("Checkout retry"), 1)
	if len(got) != 1 || got[1] != 1 {
		t.Fatalf("got %v, want item 1 as best match", got)
	}
}

func TestSampleKeepsErrorsAndEnds(t *testing.T) {
	var items []any
	for i := range 100 {
		status := "ok"
		if i == 42 {
			status = "failed"
		}
		items = append(items, map[string]any{"i": json.Number(strconv.Itoa(i)), "status": status})
	}
	got := Sample(items, Options{MaxItems: 6, HeadShare: 0.3, TailShare: 0.15, AnomalyStd: 2})
	var kept []string
	for _, it := range got {
		if m, ok := it.(map[string]any); ok {
			kept = append(kept, string(m["i"].(json.Number)))
		}
	}
	for _, want := range []string{"0", "42", "99"} {
		if !slices.Contains(kept, want) {
			t.Errorf("item %s dropped; kept %v", want, kept)
		}
	}
	if len(kept) > 7 {
		t.Errorf("kept %d items, want about 6", len(kept))
	}
}
