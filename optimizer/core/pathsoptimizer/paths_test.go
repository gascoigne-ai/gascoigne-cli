package pathsoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/payload"
)

const trace = `--- FAIL: TestEncode (0.00s)
panic: boom
	/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2/router/router.go:42 +0x1f
	/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2/router/match.go:17 +0x2a
	/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2/server.go:88 +0x3b
	/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2/server.go:91 +0x3b
	/home/runner/work/shop/shop/internal/orders/orders.go:14
	/home/runner/work/shop/shop/internal/orders/orders_test.go:6
	/home/runner/work/shop/shop/internal/orders/orders_test.go:9
`

func optimize(t *testing.T, typ, text string) *payload.Payload {
	t.Helper()
	p := payload.New(payload.CmdOutput, text)
	p.Blocks[0].Type = typ
	if err := (&Optimizer{cfg: defaults()}).Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAliasesRepeatedPrefixes(t *testing.T) {
	got := optimize(t, "test/go", trace).Text()
	want := `[paths: §1=/home/runner/go/pkg/mod/github.com/acme/api@v1.4.2 §2=/home/runner/work/shop/shop/internal/orders]
--- FAIL: TestEncode (0.00s)
panic: boom
	§1/router/router.go:42 +0x1f
	§1/router/match.go:17 +0x2a
	§1/server.go:88 +0x3b
	§1/server.go:91 +0x3b
	§2/orders.go:14
	§2/orders_test.go:6
	§2/orders_test.go:9
`
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestLegendRestoresPaths(t *testing.T) {
	got := optimize(t, "test/go", trace).Text()
	legend, body, _ := strings.Cut(got, "\n")
	var pairs []string
	for _, kv := range strings.Fields(strings.TrimSuffix(strings.TrimPrefix(legend, legendPrefix), "]")) {
		id, path, _ := strings.Cut(kv, "=")
		pairs = append(pairs, id+"/", path+"/")
	}
	if restored := strings.NewReplacer(pairs...).Replace(body); restored != trace {
		t.Fatalf("legend does not restore the paths:\n%s", restored)
	}
}

func TestLeavesAlone(t *testing.T) {
	tests := []struct{ name, typ, text string }{
		{"source file", "fs/code", trace},
		{"data", "data/json", trace},
		{"few repeats", "test/go", "a /home/runner/work/shop/shop/internal/x.go:1\nb /home/runner/work/shop/shop/internal/y.go:2\n"},
		{"short prefixes", "build/go", strings.Repeat("./cmd/app/main.go:3: undefined: x\n", 20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optimize(t, tt.typ, tt.text).Text(); got != tt.text {
				t.Fatalf("changed:\n%s", got)
			}
		})
	}
}

func TestIdempotent(t *testing.T) {
	once := optimize(t, "test/go", trace).Text()
	if twice := optimize(t, "test/go", once).Text(); twice != once {
		t.Fatalf("second run changed:\n%s", twice)
	}
}
