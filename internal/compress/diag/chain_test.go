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
