package inproc

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// opts keeps a test away from the user's config file and ledger.
func opts(t *testing.T) Options {
	t.Helper()
	return Options{
		IgnoreUserFile: true,
		Config:         "extensions:\n  ledger:\n    path: " + filepath.Join(t.TempDir(), "l.db") + "\n",
	}
}

func TestOptimizeKeepsOriginalReachable(t *testing.T) {
	ctx := context.Background()
	e, err := New(ctx, opts(t))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close(ctx)
	raw := strings.Repeat("INFO request served path=/api/items status=200 ms=12\n", 400)
	res, err := e.Optimize(ctx, Request{Text: raw, Tool: "bash", Cmd: "tail -n 400 app.log"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OutTokens >= res.RawTokens || res.RawID == "" {
		t.Fatalf("not optimized: %+v", res)
	}
	got, err := e.Raw(ctx, res.RawID)
	if err != nil || got != raw {
		t.Fatalf("Raw = %d bytes, %v; want the original", len(got), err)
	}
	if _, err := e.Raw(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("unknown id: %v", err)
	}
}

func TestHostConfigMergesOverDefaults(t *testing.T) {
	o := opts(t)
	o.Config += "optimizers:\n  grep:\n    max_per_file: 7\n"
	data, err := ConfigYAML(o)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "max_per_file: 7") || !strings.Contains(s, "min_chars") {
		t.Fatalf("merge lost defaults or override:\n%s", s)
	}
	if _, err := ConfigYAML(Options{IgnoreUserFile: true, Config: "a: [unclosed\n"}); err == nil {
		t.Fatal("bad host config accepted")
	}
	if _, err := New(context.Background(), Options{IgnoreUserFile: true, Set: []string{"no-equals"}}); err == nil {
		t.Fatal("bad set accepted")
	}
}
