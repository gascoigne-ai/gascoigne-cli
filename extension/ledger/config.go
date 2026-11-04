package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	// DefaultRawTTL keeps originals a month, long enough to look into a run later.
	DefaultRawTTL = 30 * 24 * time.Hour
	// DefaultStatsTTL keeps the daily per-tool MCP stats a year.
	DefaultStatsTTL = 365 * 24 * time.Hour
	// DefaultEventsTTL keeps runs, usage and failure history a year.
	DefaultEventsTTL = 365 * 24 * time.Hour
)

// MemoryPath names the in-memory database: as Config.Path or $GASCOIGNE_LEDGER
// it selects memory mode, and Ledger.Path reports it.
const MemoryPath = ":memory:"

// DefaultMemoryMaxBytes caps the originals an in-memory ledger holds (gzip size).
const DefaultMemoryMaxBytes = 64 << 20

// EnvPath overrides the configured path; eval uses it to isolate runs.
const EnvPath = "GASCOIGNE_LEDGER"

type Config struct {
	// Path to the sqlite db; env vars are expanded. Default: $XDG_DATA_HOME/gascoigne/ledger.db.
	Path string `yaml:"path"`
	// Memory keeps the whole ledger in this process (same as Path ":memory:"):
	// nothing is written to disk and everything is gone at exit. For embedders
	// such as the SDK that only want the numbers of the current run.
	Memory bool `yaml:"memory"`
	// MemoryMaxBytes caps the stored originals in memory mode; the oldest go
	// first. 0 means DefaultMemoryMaxBytes. Ignored for a file ledger.
	MemoryMaxBytes int64 `yaml:"memory_max_bytes"`
	// RawTTL is how long originals stay retrievable. 0 keeps them forever.
	RawTTL time.Duration `yaml:"raw_ttl"`
	// StatsTTL is how long the daily per-tool MCP stats stay. 0 keeps them forever.
	StatsTTL time.Duration `yaml:"stats_ttl"`
	// EventsTTL is how long runs (with their steps), API usage and failure
	// history stay. 0 keeps them forever.
	EventsTTL time.Duration `yaml:"events_ttl"`
	// RollupTTL is how long the daily totals of runs pruned at EventsTTL stay
	// (by day, agent, model, command, route, outcome and optimizer). 0 keeps
	// them forever.
	RollupTTL time.Duration `yaml:"rollup_ttl"`
	// RecoverAfter is how many optimized runs of a source in a row, without
	// the agent fetching the original, forgive one of its failures, taking it
	// back toward the normal route. 0 keeps failures until events_ttl or
	// `gascoigne forget`.
	RecoverAfter int `yaml:"recover_after"`
}

func (c *Config) Validate() error {
	if c.RawTTL < 0 {
		return errors.New("raw_ttl must not be negative")
	}
	if c.StatsTTL < 0 {
		return errors.New("stats_ttl must not be negative")
	}
	if c.EventsTTL < 0 {
		return errors.New("events_ttl must not be negative")
	}
	if c.RollupTTL < 0 {
		return errors.New("rollup_ttl must not be negative")
	}
	if c.MemoryMaxBytes < 0 {
		return errors.New("memory_max_bytes must not be negative")
	}
	if c.RecoverAfter < 0 {
		return errors.New("recover_after must not be negative")
	}
	return nil
}

// File is the sqlite db the ledger opens: $GASCOIGNE_LEDGER, else Path, else
// the default.
func (c *Config) File() (string, error) {
	// Memory wins over the env path: an embedder asked for no file at all.
	if c.Memory {
		return MemoryPath, nil
	}
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

// InMemory reports whether file is the in-memory database.
func InMemory(file string) bool { return file == MemoryPath }

func (c *Config) memoryCap() int64 {
	if c.MemoryMaxBytes > 0 {
		return c.MemoryMaxBytes
	}
	return DefaultMemoryMaxBytes
}
