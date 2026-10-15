package mcptool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.go")
	src := "package a\n\n// Add sums.\nfunc Add(a, b int) int {\n\treturn a + b\n}\n\ntype T struct{}\n\nfunc (T) Run() {}\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Show(path, "Add")
	if err != nil {
		t.Fatal(err)
	}
	want := path + ":3-6 func Add\n3\t// Add sums.\n4\tfunc Add(a, b int) int {\n5\t\treturn a + b\n6\t}\n"
	if got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
	got, err = Show(path, "")
	if err != nil {
		t.Fatal(err)
	}
	want = path + ": 10 lines, 3 definitions\n4-6\tfunc Add(a, b int) int {\n8-8\ttype T struct{}\n10-10\tfunc (T) Run() {}\n"
	if got != want {
		t.Fatalf("outline got\n%q\nwant\n%q", got, want)
	}
	if _, err := Show(path, "Nope"); err == nil {
		t.Fatal("missing symbol: want error")
	}
}
