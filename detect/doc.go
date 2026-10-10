package detect

// Prose and documentation.
const (
	Markdown = "doc/markdown"
	Text     = "doc/text" // anything unrecognized
)

func init() {
	register(
		Spec{Type: Markdown, Files: []string{"*.md", "*.markdown"}, MIME: []string{"text/markdown"}},
	)
}
