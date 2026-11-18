package detect

import "testing"

func TestSpecTableSortedByPriority(t *testing.T) {
	table := specTable()
	if len(table) != len(declared) {
		t.Fatalf("table has %d specs, %d were declared", len(table), len(declared))
	}
	for i := 1; i < len(table); i++ {
		if table[i-1].Priority > table[i].Priority {
			t.Fatalf("spec %d (%s, priority %d) comes before spec %d (%s, priority %d)",
				i-1, table[i-1].Type, table[i-1].Priority, i, table[i].Type, table[i].Priority)
		}
	}
}
