package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const DefaultRawTTL = 24 * time.Hour

// EnvPath overrides the configured path; eval uses it to isolate runs.
const EnvPath = "GASCOIGNE_LEDGER"

type Config struct {
	// Path to the sqlite db; env vars are expanded. Default: $XDG_DATA_HOME/gascoigne/ledger.db.
	Path string `yaml:"path"`
	// RawTTL is how long originals stay retrievable. 0 keeps them forever.
	RawTTL time.Duration `yaml:"raw_ttl"`
}

func (c *Config) Validate() error {
	if c.RawTTL < 0 {
		return errors.New("raw_ttl must not be negative")
	}
	return nil
}

func (c *Config) path() (string, error) {
	if p := os.Getenv(EnvPath); p != "" {
		return p, nil
	}
	if c.Path != "" {
		return os.ExpandEnv(c.Path), nil
	}
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "gascoigne", "ledger.db"), nil
}
