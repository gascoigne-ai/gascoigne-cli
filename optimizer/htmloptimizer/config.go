package htmloptimizer

// Config sets whether links survive the conversion to markdown.
type Config struct {
	// Links keeps hrefs as [text](url); false keeps link text only.
	Links bool `yaml:"links"`
}
