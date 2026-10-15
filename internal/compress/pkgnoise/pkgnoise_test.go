package pkgnoise

import (
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

func TestCompress(t *testing.T) {
	r, _ := For("npm")
	in := "npm http fetch GET 200 x\nnpm warn deprecated a@1: old\nnpm warn deprecated b@2: older\n\nadded 3 packages in 1s\nnpm ERR! code E404\n"
	want := "npm warn deprecated (2): a@1, b@2\n\nadded 3 packages in 1s\nnpm ERR! code E404\n"
	if got := compress.Join(Compress(compress.Lines(in), r)); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
	if _, ok := For("nope"); ok {
		t.Fatal("unknown manager accepted")
	}
}

func TestEveryRuleCompiles(t *testing.T) {
	for _, d := range []string{"npm", "pnpm", "yarn", "bun", "pip", "uv", "poetry", "go-mod", "cargo", "os"} {
		if _, ok := For(d); !ok {
			t.Errorf("no rules for %s", d)
		}
	}
}
