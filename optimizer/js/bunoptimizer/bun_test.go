package bunoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.BunInstall}}}
	Optimizer{}.Optimize(nil, p)
	return p.Blocks[0].Text
}

func TestInstall(t *testing.T) {
	in := "bun install v1.2.21\nResolving dependencies\n[12.30ms] Downloaded ms tarball\nExtracted ms@2.1.3\nSaved lockfile\n\n+ hono@4.9.6\n\n482 packages installed [3.18s]\n"
	want := "bun install v1.2.21\n\n+ hono@4.9.6\n\n482 packages installed [3.18s]\n"
	if got := run(in); got != want {
		t.Fatalf("got %q", got)
	}
}

func TestTest(t *testing.T) {
	in := "bun test v1.2.21\n\na.test.ts:\n✓ a > ok [0.1ms]\nerror: expect(received).toBe(expected)\n✗ a > bad [0.2ms]\n\n 1 pass\n 1 fail\n"
	got := run(in)
	for _, want := range []string{"✗ a > bad", "error: expect", " 1 fail", "1 passing test line"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "✓") || strings.Contains(got, "a.test.ts:") {
		t.Errorf("kept passing noise:\n%s", got)
	}
}
