package detect

import "slices"

// Prose and documentation.
const (
	Markdown = "doc/markdown"
	Man      = "doc/man"
	Help     = "doc/help" // --help and help subcommands
	RST      = "doc/rst"
	Text     = "doc/text" // anything unrecognized
)

func init() {
	register(
		Spec{Type: Markdown, Files: []string{"*.md", "*.markdown", "*.mdx"}, MIME: []string{"text/markdown"}, Cmds: []string{"glow", "gh repo view"}, Priority: 88,
			Sniff: all(anyLine(`^#{1,4} \S`), anyLine("^```|\\]\\((https?://|#|\\./)|^\\s*[-*] \\[[ x]\\] |^\\|.*\\|$"))},
		Spec{Type: Man, Cmds: []string{"man"}, Priority: 40,
			Sniff: all(anyLine(`^NAME$`), anyLine(`^SYNOPSIS$`), anyLine(`^\S+\(\d\w*\)\s.*\S+\(\d\w*\)$`))},
		// -h is not help everywhere (df -h, du -h, sort -h).
		Spec{Type: Help, Priority: 87,
			CmdFunc: func(args []string) bool {
				return slices.Contains(args[1:], "--help") || len(args) > 1 && args[1] == "help"
			},
			Sniff: all(anyLine(`^(Usage|USAGE|usage):`), anyLine(`^\s{1,8}-{1,2}\w[\w-]*(,|\s|=|$)`))},
		Spec{Type: RST, Files: []string{"*.rst"}, MIME: []string{"text/x-rst"}, Priority: 89,
			Sniff: all(anyLine(`^\.\. [\w-]+::`), anyLine(`^(={3,}|-{3,}|~{3,})$`))},
	)
}
