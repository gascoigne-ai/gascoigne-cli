package lsoptimizer

import (
	"testing"

	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(cfg *Config, in string) string {
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: detect.LS}}}
	(&Optimizer{cfg: cfg}).Optimize(nil, p)
	return p.Blocks[0].Text
}

func TestRewrite(t *testing.T) {
	in := `total 24
drwxr-xr-x  5 u g  4096 Oct 10 01:04 .
drwxr-xr-x  3 u g  4096 Oct 10 01:04 ..
drwxr-xr-x  2 u g  4096 Oct  9  2025 doc
-rw-r--r--  1 u g 12293 Oct 10 01:04 a.go
-rwxr-xr-x  1 u g   512 Oct 10 01:04 run.sh
lrwxrwxrwx  1 u g     6 Oct 10 01:04 latest -> v1.2.3
`
	want := "dirs: doc/\nfiles (3): a.go 12K  run.sh* 512  latest@ -> v1.2.3\n"
	if got := run(&Config{SizesUpTo: 60, Width: 120}, in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestMultipleDirsAndNoSizes(t *testing.T) {
	in := "a:\ntotal 0\n-rw-r--r-- 1 u g 1 Oct 1 12:00 x\n\nb:\ntotal 0\n-rw-r--r-- 1 u g 1 Oct 1 12:00 y\n-rw-r--r-- 1 u g 1 2026-10-01 12:00 z\n"
	want := "a:\nfiles (1): x\nb:\nfiles (2): y  z\n"
	if got := run(&Config{SizesUpTo: 0, Width: 120}, in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestShortFormatUntouched(t *testing.T) {
	in := "a.go  b.go  doc\n"
	if got := run(&Config{SizesUpTo: 60, Width: 120}, in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestHuman(t *testing.T) {
	for in, want := range map[string]string{"0": "0", "1023": "1023", "1024": "1K", "1536": "1.5K", "1048576": "1M", "4.0K": "4.0K"} {
		if got := human(in); got != want {
			t.Errorf("human(%s) = %s, want %s", in, got, want)
		}
	}
}
