package diag

import (
	"slices"
	"strings"
	"testing"
)

func TestCollapse(t *testing.T) {
	var in []string
	in = append(in, "In file included from a.h:3,", "                 from b.h:5,", "                 from c.h:7,", "                 from d.h:9,", "                 from e.h:11,", "                 from main.cc:1:")
	in = append(in, "main.cc: In instantiation of 'struct Box<0>':")
	for i := 1; i <= 8; i++ {
		in = append(in, "main.cc:20:12:   recursively required from 'struct Box<"+string(rune('0'+i))+">'")
	}
	in = append(in, "main.cc:20:12:   required from here", "main.cc:5:7: error: static assertion failed")
	got := collapse(in, 2)
	want := []string{
		"In file included from a.h:3,", "                 from b.h:5,",
		"… 3 include chain lines omitted",
		"                 from main.cc:1:",
		"main.cc: In instantiation of 'struct Box<0>':",
		"main.cc:20:12:   recursively required from 'struct Box<1>'",
		"… 7 template instantiation notes omitted",
		"main.cc:20:12:   required from here",
		"main.cc:5:7: error: static assertion failed",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got:\n%s", strings.Join(got, "\n"))
	}
}

func TestFoldUndefined(t *testing.T) {
	in := []string{
		"/usr/bin/ld: main.o: in function `main':",
		"main.cc:(.text+0x12): undefined reference to `net::connect(int)'",
		"main.cc:(.text+0x20): undefined reference to `net::close(int)'",
		"/usr/bin/ld: db.o: in function `db::open()':",
		"db.cc:(.text+0x8): undefined reference to `net::connect(int)'",
		"db.cc:(.text+0x9): undefined reference to `sqlite3_open'",
		"collect2: error: ld returned 1 exit status",
	}
	got := foldUndefined(in)
	want := []string{
		"undefined reference (3 symbols): net::connect(int), net::close(int), sqlite3_open",
		"collect2: error: ld returned 1 exit status",
	}
	if !slices.Equal(got, want) {
		t.Errorf("got:\n%s", strings.Join(got, "\n"))
	}
	if again := foldUndefined(got); !slices.Equal(again, got) {
		t.Errorf("not idempotent: %v", again)
	}
}

// Real clang++ output for a recursive template that trips a static_assert.
func TestCollapseClangNotes(t *testing.T) {
	in := []string{
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<16>' requested here",
		"    5 |     Box<N + 1> inner;",
		"      |                ^",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<15>' requested here",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<14>' requested here",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<13>' requested here",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<12>' requested here",
		"tmpl.cc:5:16: note: (skipping 7 contexts in backtrace; use -ftemplate-backtrace-limit=0 to see all)",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<4>' requested here",
		"tmpl.cc:5:16: note: in instantiation of template class 'Box<3>' requested here",
		"tmpl.cc:8:12: note: in instantiation of template class 'Box<0>' requested here",
		"    8 |     Box<0> b;",
	}
	got := collapse(in, 2)
	want := []string{in[0], in[1], in[2], in[3], in[4], "… 5 template instantiation notes omitted", in[10], in[11]}
	if !slices.Equal(got, want) {
		t.Errorf("got:\n%s", strings.Join(got, "\n"))
	}
}

// Real g++ and ld output of a link with missing symbols: only the first
// line has the "/usr/bin/ld: file.o: in function" context.
func TestFoldUndefinedReal(t *testing.T) {
	in := []string{
		"/usr/bin/ld: CMakeFiles/app.dir/src/main.cc.o: in function `main':",
		"main.cc:(.text+0xa): undefined reference to `net::connect(int)'",
		"/usr/bin/ld: main.cc:(.text+0x14): undefined reference to `net::close(int)'",
		"/usr/bin/ld: main.cc:(.text+0x23): undefined reference to `sqlite3_open(char const*)'",
		"collect2: error: ld returned 1 exit status",
	}
	want := []string{"undefined reference (3 symbols): net::connect(int), net::close(int), sqlite3_open(char const*)", in[4]}
	if got := foldUndefined(in); !slices.Equal(got, want) {
		t.Errorf("got:\n%s", strings.Join(got, "\n"))
	}
}
