package stdoutexporter

type Config struct {
	// Hint appends how to fetch the original when output was optimized.
	Hint bool `yaml:"hint"`
}
