package lsoptimizer

import "testing"

// find -ls puts the inode and block count before the mode; the entry
// pattern anchored on the mode, so the listing passed through unparsed.
func TestFindLs(t *testing.T) {
	in := "28576243      0 drwxr-xr-x   1 user user        84 Nov  1 02:58 optimizer/lsoptimizer\n" +
		"28576244      4 -rw-r--r--   1 user user       588 Oct 16 01:16 optimizer/lsoptimizer/config.go\n" +
		"28576247      4 -rwxr-xr-x   1 user user     12293 Oct 16 01:16 optimizer/lsoptimizer/run.sh\n"
	want := "dirs: optimizer/lsoptimizer/\nfiles (2): optimizer/lsoptimizer/config.go 588  optimizer/lsoptimizer/run.sh* 12K\n"
	if got := run(&Config{SizesUpTo: 60, Width: 120}, in); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
