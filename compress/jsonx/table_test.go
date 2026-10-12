package jsonx

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTableFlattensNestedObjects(t *testing.T) {
	var items []any
	for i := range 6 {
		// Subtrees differ per item in one leaf each: whole subtrees cannot
		// hoist, their equal leaves can once flattened.
		items = append(items, map[string]any{
			"name":  "svc-" + string(rune('a'+i)),
			"owner": map[string]any{"team": "platform", "oncall": "pager-" + string(rune('1'+i)), "region": "eu-west-1"},
			"spec":  map[string]any{"replicas": json.Number(string(rune('1' + i))), "image": "registry.example/app:1.4.2"},
		})
	}
	c := &crusher{cfg: Default()}
	tbl, ok := c.table(items)
	if !ok {
		t.Fatal("no table")
	}
	out, _ := json.Marshal(tbl)
	got := string(out)
	for _, want := range []string{`"owner.team":"platform"`, `"spec.image":"registry.example/app:1.4.2"`} {
		if !strings.Contains(got, want) {
			t.Errorf("nested leaves equal in every item should hoist into _same: want %s in\n%s", want, got)
		}
	}
	if strings.Contains(got, `{"oncall"`) {
		t.Errorf("nested objects repeated per row:\n%s", got)
	}
}

func TestTableKeepsNestedWhenFlatIsLarger(t *testing.T) {
	var items []any
	for i := range 4 {
		// Every nested value differs: flat columns only add long dotted names.
		items = append(items, map[string]any{"id": json.Number(string(rune('1' + i))), "meta": map[string]any{"x": string(rune('a' + i))}})
	}
	c := &crusher{cfg: Default()}
	plain, _ := c.plainTable(items)
	tbl, _ := c.table(items)
	if size(tbl) > size(plain) {
		t.Fatalf("picked the larger table: %d > %d", size(tbl), size(plain))
	}
}

func TestTableLeavesArrayItemsAlone(t *testing.T) {
	var items []any
	for i := range 40 {
		items = append(items, map[string]any{"id": json.Number(strings.Repeat("1", i%5+1)), "name": "item-" + strings.Repeat("x", 10) + string(rune('a'+i%26))})
	}
	items = append(items, []any{"a", "b"})
	c := &crusher{cfg: Default()}
	if tbl, ok := c.table(items); ok {
		t.Fatalf("an array item would read as a row: %v", tbl)
	}
}
