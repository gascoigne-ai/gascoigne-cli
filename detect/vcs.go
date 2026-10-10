package detect

// Version control and forges.
const (
	Diff      = "vcs/git-diff" // also git show, patch files
	GitStatus = "vcs/git-status"
	GitLog    = "vcs/git-log"
	GitRemote = "vcs/git-remote" // push, pull, fetch, clone
)

func init() {
	register(
		Spec{Type: Diff, Cmds: []string{"git diff", "git show"}, Files: []string{"*.diff", "*.patch"}, Priority: 40,
			Sniff: anyLine(`^diff --git |^@@ -\d+(,\d+)? \+\d+(,\d+)? @@`)},
		Spec{Type: GitLog, Cmds: []string{"git log"}, Priority: 41, Sniff: anyLine(`^commit [0-9a-f]{40}`)},
		Spec{Type: GitStatus, Cmds: []string{"git status"}, Priority: 42,
			Sniff: anyLine(`^On branch |^HEAD detached |^Changes (not staged|to be committed)|^Untracked files:|^nothing to commit`)},
		Spec{Type: GitRemote, Cmds: []string{"git push", "git pull", "git fetch", "git clone"}},
	)
}
