package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type found struct {
	line int
	name string
}

func foundIn(t *testing.T, src string) []found {
	t.Helper()
	violations, err := check("x.go", []byte(src))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	var got []found
	for _, violation := range violations {
		got = append(got, found{violation.Line, violation.Name})
	}
	return got
}

func TestCheckReportsOneLetterParameters(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []found
	}{
		{"declaration", "package p\nfunc f(x int) {}\n", []found{{2, "x"}}},
		{"long name ok", "package p\nfunc f(value int) {}\n", nil},
		{"method params", "package p\ntype C struct{}\nfunc (c C) m(a int) {}\n", []found{{3, "a"}}},
		{"closure", "package p\nvar g = func(s string) {}\n", []found{{2, "s"}}},
		{"grouped", "package p\nfunc f(a, b int) {}\n", []found{{2, "a"}, {2, "b"}}},
		{"line of identifier", "package p\nfunc f(\n\tok int,\n\tz int,\n) {}\n", []found{{4, "z"}}},
		{"func typed field", "package p\ntype S struct{ cb func(n int) }\n", []found{{2, "n"}}},
		{"interface method", "package p\ntype I interface{ M(q int) }\n", []found{{2, "q"}}},
		{"func type decl", "package p\ntype H func(r int)\n", []found{{2, "r"}}},
		{"results not checked", "package p\nfunc f() (n int) { return }\n", nil},
		{"loop index ok", "package p\nfunc f() {\n\tfor i := 0; i < 3; i++ {\n\t}\n}\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := foundIn(t, c.src); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestCheckExemptions(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []found
	}{
		{"blank ok", "package p\nfunc f(_ int) {}\n", nil},
		{"receiver ok", "package p\ntype C struct{}\nfunc (c C) m() {}\n", nil},
		{"receiver pointer ok", "package p\ntype C struct{}\nfunc (c *C) m() {}\n", nil},
		{"type params ok", "package p\nfunc f[T any, K comparable](value T, key K) {}\n", nil},
		{"t of testing.T ok", "package p\nimport \"testing\"\nfunc f(t *testing.T) {}\n", nil},
		{"b of testing.B ok", "package p\nimport \"testing\"\nfunc f(b *testing.B) {}\n", nil},
		{"t of testing.TB ok", "package p\nimport \"testing\"\nfunc f(t testing.TB) {}\n", nil},
		{"t of testing.F ok", "package p\nimport \"testing\"\nfunc f(t *testing.F) {}\n", nil},
		{"f of testing.F bad", "package p\nimport \"testing\"\nfunc f(f *testing.F) {}\n", []found{{3, "f"}}},
		{"x of testing.T bad", "package p\nimport \"testing\"\nfunc f(x *testing.T) {}\n", []found{{3, "x"}}},
		{"t of other.T bad", "package p\nimport \"other\"\nfunc f(t *other.T) {}\n", []found{{3, "t"}}},
		{"t of local T bad", "package p\ntype T int\nfunc f(t *T) {}\n", []found{{3, "t"}}},
		{"t int bad", "package p\nfunc f(t int) {}\n", []found{{2, "t"}}},
		{"no testing import bad", "package p\nfunc f(t *testing.T) {}\n", []found{{2, "t"}}},
		{"aliased testing ok", "package p\nimport tst \"testing\"\nfunc f(t *tst.T) {}\n", nil},
		{"aliased testing, plain name bad", "package p\nimport tst \"testing\"\nfunc f(t *testing.T) {}\n", []found{{3, "t"}}},
		{"closure t ok", "package p\nimport \"testing\"\nvar g = func(t *testing.T) {}\n", nil},
		{"fuzz reports s only", "package p\nimport \"testing\"\nvar g = func(t *testing.T, s string) {}\n", []found{{3, "s"}}},
		{"method params still checked", "package p\ntype C struct{}\nfunc (c C) m(a int, _ int) {}\n", []found{{3, "a"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := foundIn(t, c.src); !slices.Equal(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestCountsSortedByPackage(t *testing.T) {
	a := Violation{File: "b/x.go", Line: 1, Name: "a", Package: "b"}
	b := Violation{File: "a/y.go", Line: 1, Name: "b", Package: "a"}
	c := Violation{File: "b/z.go", Line: 2, Name: "c", Package: "b"}
	want := []PackageCount{{"a", 1}, {"b", 2}}
	for _, input := range [][]Violation{{a, b, c}, {c, b, a}, {b, a, c}} {
		if got := counts(input); !slices.Equal(got, want) {
			t.Errorf("counts(%v) = %v, want %v", input, got, want)
		}
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, src := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

const bad = "package p\nfunc f(x int) {}\n"

func packagesOf(violations []Violation, root string) []string {
	var got []string
	for _, violation := range violations {
		rel, _ := filepath.Rel(root, violation.File)
		got = append(got, violation.Package[len(root):]+"|"+filepath.ToSlash(rel))
	}
	slices.Sort(got)
	return got
}

func TestCollectWalksPaths(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.go":           bad,
		"a/a_test.go":      "package p_test\nfunc g(y int) {}\n",
		"a/sub/s.go":       bad,
		"a/testdata/t.go":  bad,
		"a/vendor/v.go":    bad,
		"a/.hidden/h.go":   bad,
		"a/notes.txt":      "func f(x int)",
		"b/b.go":           bad,
		"broken/broken.go": "package p\nfunc (",
	})
	cases := []struct {
		name     string
		paths    []string
		want     []string
		wantErrs int
	}{
		{"dir recursive", []string{"a"}, []string{"/a/sub|a/sub/s.go", "/a|a/a.go", "/a|a/a_test.go"}, 0},
		{"dir with dots", []string{"a/..."}, []string{"/a/sub|a/sub/s.go", "/a|a/a.go", "/a|a/a_test.go"}, 0},
		{"single file", []string{"b/b.go"}, []string{"/b|b/b.go"}, 0},
		{"unparsable reported, others count", []string{"broken", "b"}, []string{"/b|b/b.go"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths := make([]string, len(c.paths))
			for i, path := range c.paths {
				// Join would clean "a/..." into something else, so concatenate.
				paths[i] = root + "/" + path
			}
			violations, errs := collect(paths)
			if got := packagesOf(violations, root); !slices.Equal(got, c.want) {
				t.Errorf("violations %v, want %v", got, c.want)
			}
			if len(errs) != c.wantErrs {
				t.Errorf("errors %v, want %d", errs, c.wantErrs)
			}
		})
	}
}

func TestRunExitCodes(t *testing.T) {
	root := writeTree(t, map[string]string{
		"a/a.go":           "package p\nfunc f(x int, y int) {}\n",
		"b/b.go":           bad,
		"clean/c.go":       "package p\nfunc f(value int) {}\n",
		"broken/broken.go": "package p\nfunc (",
	})
	dir := func(name string) string { return filepath.Join(root, name) }
	cases := []struct {
		name       string
		args       []string
		wantCode   int
		wantOut    string
		wantStderr string
	}{
		{"violations exit 0", []string{dir("a"), dir("b")}, 0,
			fmt.Sprintf("%s\t2\n%s\t1\ntotal\t3\n", dir("a"), dir("b")), ""},
		{"clean", []string{dir("clean")}, 0, "total\t0\n", ""},
		{"verbose details first", []string{"-v", dir("a")}, 0,
			fmt.Sprintf("%s:2: x\n%s:2: y\n%s\t2\ntotal\t2\n", filepath.Join(dir("a"), "a.go"), filepath.Join(dir("a"), "a.go"), dir("a")), ""},
		{"parse error exit 1", []string{dir("broken"), dir("b")}, 1,
			fmt.Sprintf("%s\t1\ntotal\t1\n", dir("b")), "paramnames: " + filepath.Join(dir("broken"), "broken.go") + ": "},
		{"no args exit 2", nil, 2, "", "usage: paramnames"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(c.args, &stdout, &stderr)
			if code != c.wantCode {
				t.Errorf("exit %d, want %d", code, c.wantCode)
			}
			if stdout.String() != c.wantOut {
				t.Errorf("stdout %q, want %q", stdout.String(), c.wantOut)
			}
			if !strings.Contains(stderr.String(), c.wantStderr) || (c.wantStderr == "" && stderr.Len() > 0) {
				t.Errorf("stderr %q, want to contain %q", stderr.String(), c.wantStderr)
			}
		})
	}
}
