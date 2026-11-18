package stdoutexporter

// Config sets whether the hint for fetching the original is written with the text.
type Config struct {
	// Hint appends how to fetch the original when output was optimized.
	Hint bool `yaml:"hint"`
}
