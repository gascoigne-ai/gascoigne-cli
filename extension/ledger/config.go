package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultRawTTL = 24 * time.Hour
	// DefaultStatsTTL is two weeks: a weekly review compares a week with the one before.
	DefaultStatsTTL = 14 * 24 * time.Hour
)

// EnvPath overrides the configured path; eval uses it to isolate runs.
const EnvPath = "GASCOIGNE_LEDGER"

type Config struct {
	// Path to the sqlite db; env vars are expanded. Default: $XDG_DATA_HOME/gascoigne/ledger.db.
	Path string `yaml:"path"`
	// RawTTL is how long originals stay retrievable. 0 keeps them forever.
	RawTTL time.Duration `yaml:"raw_ttl"`
	// StatsTTL is how long the daily per-tool MCP stats stay. 0 keeps them forever.
	StatsTTL time.Duration `yaml:"stats_ttl"`
}

func (c *Config) Validate() error {
	if c.RawTTL < 0 {
		return errors.New("raw_ttl must not be negative")
	}
	if c.StatsTTL < 0 {
		return errors.New("stats_ttl must not be negative")
	}
	return nil
}

// File is the sqlite db the ledger opens: $GASCOIGNE_LEDGER, else Path, else
// the default.
func (c *Config) File() (string, error) {
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
