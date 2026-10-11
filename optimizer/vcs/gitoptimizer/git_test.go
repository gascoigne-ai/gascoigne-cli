package gitoptimizer

import (
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/compress"
	"github.com/gascoigne-ai/gascoigne-cli/detect"
	"github.com/gascoigne-ai/gascoigne-cli/payload"
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

// TestLogKeepsFileLists is a regression test: --stat, --name-only and
// --name-status lines used to be dropped with the commit headers.
func TestLogKeepsFileLists(t *testing.T) {
	head := "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: Ada <ada@example.com>\nDate:   Sat Jul 11 06:13:07 2026 +0530\n\n    feat: mcp\n\n"
	tests := []struct{ name, files, want string }{
		{"stat",
			" go.sum                             |  16 ++++\n receiver/mcpreceiver/mcp.go        | 140 +++++++++++++++++\n .harness/scripts/check-imports.sh  |   2 +-\n assets/logo.png                    | Bin 0 -> 1234 bytes\n 4 files changed, 157 insertions(+), 1 deletion(-)\n",
			"  go.sum | 16 +\n  receiver/mcpreceiver/mcp.go | 140 +\n  .harness/scripts/check-imports.sh | 2 +-\n  assets/logo.png | Bin 0 -> 1234 bytes\n  4 files changed, 157 insertions(+), 1 deletion(-)\n"},
		{"name-status", "M\tgo.sum\nA\treceiver/mcpreceiver/mcp.go\n", "  M\tgo.sum\n  A\treceiver/mcpreceiver/mcp.go\n"},
		{"name-only", "go.sum\nreceiver/mcpreceiver/mcp.go\n", "  go.sum\n  receiver/mcpreceiver/mcp.go\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := "0123456 2026-07-11 Ada: feat: mcp\n" + tt.want
			if got := run(t, detect.GitLog, head+tt.files); got != want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, want)
			}
		})
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

func TestWordDiff(t *testing.T) {
	tmpl := "`Usage:{{if .Runnable}} {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}"
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{"one word changed",
			[]string{"-var defaultUsageTemplate = " + tmpl, "+const defaultUsageTemplate = " + tmpl},
			[]string{"~[-var-]{+const+} defaultUsageTemplate = " + tmpl}},
		{"middle of the line",
			[]string{`-    set -l requestComp "%[10]s=0 $args[1] %[3]s $args[2..-1] $lastArg"`,
				`+    set -l requestComp "%[10]s=0 $args[1] %[3]s $(escape $args[2..-1]) $lastArg"`},
			[]string{`~    set -l requestComp "%[10]s=0 $args[1] %[3]s [-$args[2..-1]-]{+$(escape $args[2..-1])+} $lastArg"`}},
		{"pairs in a block",
			[]string{"-var minUsagePadding = 25 // keep in sync with the template", "-var minNamePadding = 11 // keep in sync with the template",
				"+const minUsagePadding = 25 // keep in sync with the template", "+const minNamePadding = 11 // keep in sync with the template"},
			[]string{"~[-var-]{+const+} minUsagePadding = 25 // keep in sync with the template", "~[-var-]{+const+} minNamePadding = 11 // keep in sync with the template"}},
		{"mostly different stays", []string{`-	"gopkg.in/yaml.v3"`, `+	"go.yaml.in/yaml/v3"`}, nil},
		{"too short to pay", []string{"-d", "+D"}, nil},
		{"uneven block stays", []string{"-a long line that changes a bit here", "+a long line that changed a bit here", "+and another"}, nil},
		{"markers already in the text stay", []string{"-x := m[-1] // a long comment to make it pay", "+y := m[-1] // a long comment to make it pay"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.want
			if want == nil {
				want = tt.in
			}
			if got := wordDiff(tt.in); strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Fatalf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestDiffFoldsLockfilesAndGenerated(t *testing.T) {
	in := "diff --git a/go.sum b/go.sum\nindex 1..2 100644\n--- a/go.sum\n+++ b/go.sum\n@@ -1,3 +1,3 @@\n-a v1 h1:x=\n-b v1 h1:y=\n+a v2 h1:z=\n c v1 h1:w=\n" +
		"diff --git a/api/x.pb.go b/api/x.pb.go\n--- a/api/x.pb.go\n+++ b/api/x.pb.go\n@@ -1 +1 @@\n-var a = 1\n+var a = 2\n" +
		"diff --git a/gen.go b/gen.go\nnew file mode 100644\n--- /dev/null\n+++ b/gen.go\n@@ -0,0 +1,2 @@\n+// Code generated by stringer; DO NOT EDIT.\n+package x\n" +
		"diff --git a/go.mod b/go.mod\n--- a/go.mod\n+++ b/go.mod\n@@ -1 +1 @@\n-require a v1\n+require a v2\n"
	want := "diff go.sum (lockfile) +1 -2 lines folded\n" +
		"diff api/x.pb.go (generated) +1 -1 lines folded\n" +
		"diff gen.go (new) (generated) +2 -0 lines folded\n" +
		"diff go.mod\n@@ -1 +1 @@\n-require a v1\n+require a v2\n"
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
