package lsoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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

func fsOptimizer() *Optimizer {
	return &Optimizer{cfg: &Config{SizesUpTo: 60, Width: 120, MaxPerDir: 3}}
}

func TestTree(t *testing.T) {
	in := ".\n├── a\n│   ├── 1.go\n│   ├── sub\n│   │   └── x.md\n│   ├── 2.go\n│   ├── 3.go\n│   └── 4.go\n└── README.md\n\n2 directories, 6 files\n"
	got := compress.Join(fsOptimizer().tree(compress.Lines(in)))
	want := ".\na/\n  1.go\n  sub/\n    x.md\n  2.go\n  3.go\n  … 1 more: .go ×1\nREADME.md\n\n2 directories, 6 files\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestPathsGroupByDir(t *testing.T) {
	in := "./a\n./a/1.go\n./a/2.go\n./a/3.go\n./a/4.txt\n./b/c.go\n"
	got := compress.Join(fsOptimizer().paths(compress.Lines(in)))
	if got != "a/: 1.go  2.go  3.go  … 1 more: .txt ×1\nb/: c.go\n" {
		t.Fatalf("got %q", got)
	}
}

func TestDUKeepsLargest(t *testing.T) {
	var in []string
	for i := range 10 {
		in = append(in, fmt.Sprintf("%dK\t./d%d", i+1, i))
	}
	in = append(in, "2.5M\t./big", "3.1M\t.")
	got := compress.Join(fsOptimizer().du(in))
	if !strings.HasPrefix(got, "largest:\n2.5M\t./big\n10K\t./d9\n") || !strings.HasSuffix(got, "… 5 smaller entries omitted\n3.1M\t.\n") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestDFDropsPseudo(t *testing.T) {
	in := "Filesystem      Size  Used Avail Use% Mounted on\n/dev/sda1       100G   50G   50G  50% /\ntmpfs            16G     0   16G   0% /dev/shm\n/dev/sda2       1.0G  100M  900M  10% /boot\n"
	got := compress.Join(fsOptimizer().df(compress.Lines(in)))
	if strings.Contains(got, "tmpfs") || !strings.Contains(got, "/boot") || !strings.Contains(got, "1 pseudo filesystem") {
		t.Fatalf("got:\n%s", got)
	}
}
