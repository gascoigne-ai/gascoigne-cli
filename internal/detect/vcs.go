package detect

// Version control and forges.
const (
	Diff      = "vcs/git-diff" // also git show, patch files
	GitStatus = "vcs/git-status"
	GitLog    = "vcs/git-log"
	GitRemote = "vcs/git-remote" // push, pull, fetch, clone
	GitBlame  = "vcs/git-blame"
	GitBranch = "vcs/git-branch"
	GHList    = "vcs/gh-list" // gh pr/issue/run/release/repo list
	GHView    = "vcs/gh-view" // gh pr/issue view
)

func init() {
	register(
		Spec{Type: Diff, Cmds: []string{"git diff", "git show", "git stash show -p", "gh pr diff", "glab mr diff", "jj diff --git", "hg diff --git"}, Files: []string{"*.diff", "*.patch"}, Priority: 40,
			Sniff: anyLine(`^diff --git |^@@ -\d+(,\d+)? \+\d+(,\d+)? @@`)},
		Spec{Type: GitLog, Cmds: []string{"git log"}, Priority: 41, Sniff: anyLine(`^commit [0-9a-f]{40}`)},
		Spec{Type: GitStatus, Cmds: []string{"git status"}, Priority: 42,
			Sniff: anyLine(`^On branch |^HEAD detached |^Changes (not staged|to be committed)|^Untracked files:|^nothing to commit`)},
		Spec{Type: GitRemote, Cmds: []string{"git push", "git pull", "git fetch", "git clone"}},
		Spec{Type: GitBlame, Cmds: []string{"git blame", "git annotate"}, Priority: 43,
			Sniff: ratio(`^\^?[0-9a-f]{7,40} (\S+ )?\(.+ \d{4}-\d\d-\d\d \d\d:\d\d:\d\d [+-]\d{4} +\d+\) `, 0.8)},
		Spec{Type: GitBranch, Cmds: []string{"git branch"}},
		Spec{Type: GHList, Cmds: []string{"gh pr list", "gh issue list", "gh run list", "gh release list", "gh repo list", "gh workflow list", "gh pr status", "gh search prs", "gh search issues", "gh pr checks", "gh gist list", "gh label list", "gh secret list", "gh cache list", "gh codespace list", "glab mr list", "glab issue list", "glab ci list"}},
		Spec{Type: GHView, Cmds: []string{"gh pr view", "gh issue view"}, Priority: 44,
			Sniff: all(firstLine(`^title:\t`), anyLine(`^state:\t`), anyLine(`^--$`))},
	)
}
