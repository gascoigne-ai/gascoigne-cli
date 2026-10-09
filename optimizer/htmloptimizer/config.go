package htmloptimizer

type Config struct {
	// Links keeps hrefs as [text](url); false keeps link text only.
	Links bool `yaml:"links"`
}
