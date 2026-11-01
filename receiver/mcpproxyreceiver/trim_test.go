package mcpproxyreceiver

import (
	"encoding/json"
	"testing"
)

// TestTrimDescriptions: trimming shortens descriptions only (names and schemas
// stay byte for byte) and gives the same list on every start, since the tool
// list is part of the cached prompt prefix.
func TestTrimDescriptions(t *testing.T) {
	plain := startProxy(t, realServer(8), &Config{Retrieve: "gascoigne_retrieve", CompactTools: true}, nil).tools(t)
	trim := &Config{Retrieve: "gascoigne_retrieve", CompactTools: true, TrimDescriptions: 0.5}
	first := startProxy(t, realServer(8), trim, nil).tools(t)
	again := startProxy(t, realServer(8), trim, nil).tools(t)

	if listTokens(first) >= listTokens(plain) {
		t.Fatalf("trimmed list %d tokens, plain %d", listTokens(first), listTokens(plain))
	}
	t.Logf("tool list %d → %d tokens", listTokens(plain), listTokens(first))
	marshal := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if marshal(first) != marshal(again) {
		t.Fatal("trimmed tool list differs between starts")
	}
	for i := range plain {
		if first[i].Name != plain[i].Name || marshal(first[i].InputSchema) != marshal(plain[i].InputSchema) {
			t.Fatalf("tool %s: name or schema changed", plain[i].Name)
		}
		if first[i].Name == "gascoigne_retrieve" {
			continue // added by the proxy, not trimmed
		}
		if first[i].Description == "" || first[i].Description == plain[i].Description {
			t.Fatalf("tool %s: description %q", plain[i].Name, first[i].Description)
		}
	}
}

func TestTrimDescriptionsValidate(t *testing.T) {
	for _, v := range []float64{-0.1, 1.1} {
		if (&Config{Retrieve: "r", TrimDescriptions: v}).Validate() == nil {
			t.Errorf("trim_descriptions %v accepted", v)
		}
	}
}
