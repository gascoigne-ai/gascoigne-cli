package gitoptimizer

import (
	"strings"
	"testing"

	"github.com/lukaszb/gascoigne/compress"
	"github.com/lukaszb/gascoigne/detect"
	"github.com/lukaszb/gascoigne/payload"
)

func run(t *testing.T, typ, in string) string {
	t.Helper()
	p := &payload.Payload{Blocks: []payload.Block{{Text: in, Type: typ}}}
	o := &Optimizer{cfg: &Config{ContextLines: 1, MaxFileLines: 200}}
	if err := o.Optimize(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	return p.Blocks[0].Text
}

func TestStatus(t *testing.T) {
	in := "On branch main\nChanges to be committed:\n  (use \"git restore --staged <file>...\" to unstage)\n" +
		"\tmodified:   a/x.go\n\trenamed:    old.go -> new.go\n\n" +
		"Changes not staged for commit:\n\tmodified:   d/1.go\n\tmodified:   d/2.go\n\tmodified:   d/3.go\n\tdeleted:    gone.go\n\n" +
		"Untracked files:\n  (use \"git add <file>...\" to include in what will be committed)\n\ttmp/\n\n"
	want := "On branch main\nstaged:\n  M a/x.go\n  R old.go -> new.go\nunstaged:\n  M d/{1.go,2.go,3.go}\n  D gone.go\nuntracked:\n  ? tmp/\n"
	if got := run(t, detect.GitStatus, in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestLog(t *testing.T) {
	in := `commit 0123456789abcdef0123456789abcdef01234567 (HEAD -> main, origin/main)
Author: Ada Lovelace <ada@example.com>
Date:   Sat Jul 11 06:13:07 2026 +0530

    fix: parser handles empty input

    Longer explanation.
    More.

commit 89abcdef0123456789abcdef0123456789abcdef
Merge: 0123456 1234567
Author: Bob <bob@example.com>
Date:   Fri Jul 10 10:00:00 2026 +0000

    Merge branch 'x'
`
	want := "0123456 (HEAD -> main, origin/main) 2026-07-11 Ada Lovelace: fix: parser handles empty input (+2 body lines)\n" +
		"89abcde 2026-07-10 Bob: Merge branch 'x' [merge]\n"
	if got := run(t, detect.GitLog, in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestOnelineLogUntouched(t *testing.T) {
	in := "0123456 fix a\n89abcde feat b\n"
	if got := run(t, detect.GitLog, in); got != in {
		t.Fatalf("got %q", got)
	}
}

func TestDiff(t *testing.T) {
	in := `diff --git a/x.go b/x.go
index 111..222 100644
--- a/x.go
+++ b/x.go
@@ -1,9 +1,9 @@ func f() {
 a
 b
 c
-d
+D
 e
 f
 g
 h
-i
+I
diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..333
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+hello
`
	want := `diff x.go
@@ -1,9 +1,9 @@ func f() {
 c
-d
+D
 e
 …
 h
-i
+I
diff new.txt (new)
@@ -0,0 +1 @@
+hello
`
	if got := run(t, detect.Diff, in); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestRemoteDropsProgress(t *testing.T) {
	in := "Counting objects: 100% (3/3), done.\nremote: Resolving deltas: 100% (1/1)\nremote: \nTo host:repo.git\n ! [rejected]  main -> main (fetch first)\nerror: failed to push some refs\n"
	got := run(t, detect.GitRemote, in)
	if strings.Contains(got, "Counting") || strings.Contains(got, "Resolving") {
		t.Fatalf("kept progress:\n%s", got)
	}
	for _, keep := range []string{"[rejected]", "error: failed to push"} {
		if !strings.Contains(got, keep) {
			t.Errorf("lost %q", keep)
		}
	}
}

func TestMaxFileLines(t *testing.T) {
	var b strings.Builder
	b.WriteString("diff --git a/f b/f\n@@ -0,0 +1,300 @@\n")
	for range 300 {
		b.WriteString("+x\n")
	}
	p := &payload.Payload{Blocks: []payload.Block{{Text: b.String(), Type: detect.Diff}}}
	o := &Optimizer{cfg: &Config{ContextLines: 1, MaxFileLines: 10}}
	o.Optimize(t.Context(), p)
	lines := compress.Lines(p.Blocks[0].Text)
	if len(lines) != 12 || !strings.Contains(lines[11], "291 diff lines omitted") {
		t.Fatalf("got %d lines, last %q", len(lines), lines[len(lines)-1])
	}
}

func TestBlameGroupsRuns(t *testing.T) {
	in := "54c53fae (dev 2026-10-10 01:07:38 +0200  1) package x\n54c53fae (dev 2026-10-10 01:07:38 +0200  2) \n^602c5ffb (Ann Lee 2026-09-01 02:23:02 +0200  3) func f() {}\n"
	got := compress.Join(blame(compress.Lines(in)))
	if got != "54c53fae dev 2026-10-10\n    1 package x\n    2 \n602c5ffb Ann Lee 2026-09-01\n    3 func f() {}\n" {
		t.Fatalf("got %q", got)
	}
}

func TestGHViewDropsEmptyFields(t *testing.T) {
	got := compress.Join(ghView("title:\tx\nlabels:\t\nstate:\tOPEN\n--\n<!-- template -->\nbody\n"))
	if got != "title: x\nstate: OPEN\n--\n\nbody\n" {
		t.Fatalf("got %q", got)
	}
}
