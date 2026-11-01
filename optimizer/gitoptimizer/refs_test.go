package gitoptimizer

import (
	"os"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/detect"
)

// fixture reads real command output from the eval corpus.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/eval/git/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRefs(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"reflog folds branch switches", fixture(t, "reflog.txt"), "3ee3be3 HEAD@{0}: commit: fix: thing\n" +
			"d90f38f HEAD@{1}: reset: moving to HEAD\n" +
			"d90f38f HEAD@{2..8}: checkout: 7 moves between main, feature/router\n" +
			"d90f38f HEAD@{9}: commit: feat(cart): step 3\n" +
			"99a863d HEAD@{10}: commit: feat(cart): step 2\n" +
			"d61b857 HEAD@{11}: commit (initial): feat(cart): step 1\n"},
		{"two switches stay", "d90f38f HEAD@{2}: checkout: moving from a to b\nd90f38f HEAD@{3}: checkout: moving from b to a\n",
			"d90f38f HEAD@{2}: checkout: moving from a to b\nd90f38f HEAD@{3}: checkout: moving from b to a\n"},
		{"remote -v merges equal pairs", "mirror\tgit@mirror.io:shop/shop.git (fetch)\nmirror\tgit@push.io:shop/shop.git (push)\n" +
			"origin\tgit@example.com:shop/shop.git (fetch)\norigin\tgit@example.com:shop/shop.git (push)\n",
			"mirror\tgit@mirror.io:shop/shop.git (fetch)\nmirror\tgit@push.io:shop/shop.git (push)\norigin\tgit@example.com:shop/shop.git (fetch, push)\n"},
		{"worktree padding", "/tmp/gx/shop    3ee3be3 [feature/router]\n/tmp/gx/shop-wt 3ee3be3 [wip]\n/tmp/gx/bare.git  (bare)\n",
			"/tmp/gx/shop 3ee3be3 [feature/router]\n/tmp/gx/shop-wt 3ee3be3 [wip]\n/tmp/gx/bare.git (bare)\n"},
		{"submodule status", " a2247eeaedda1825ee828162d81aa473f52ed34c .harness/common (heads/main)\n", " a2247ee .harness/common (heads/main)\n"},
		{"full shas", "3ee3be308477ce8942dc99f90ef438b31d11d94c commit\trefs/heads/main\n", "3ee3be3 commit\trefs/heads/main\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := run(t, detect.GitRefs, tt.in); got != tt.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// A bare `git diff --stat` has no "diff --git" lines; its bars are scaled to
// the terminal, so only the signs carry information.
func TestDiffStatAlone(t *testing.T) {
	want := "README.md | 2 +-\nsrc/cart.go | 80 +\n2 files changed, 81 insertions(+), 1 deletion(-)\n"
	if got := run(t, detect.Diff, diffStat); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

const diffStat = " README.md   |  2 +-\n src/cart.go | 80 +++++++++++++++++++++++++++++++++++++++++++++++++++++++++++++\n 2 files changed, 81 insertions(+), 1 deletion(-)\n"

func TestDiffStatBeforePatch(t *testing.T) {
	in := diffStat + "\ndiff --git a/README.md b/README.md\nindex 1..2 100644\n--- a/README.md\n+++ b/README.md\n@@ -1 +1,2 @@\n-2\n+3\n+4\n"
	got := run(t, detect.Diff, in)
	for _, w := range []string{"src/cart.go | 80 +\n", "diff README.md\n", "+4\n"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}
