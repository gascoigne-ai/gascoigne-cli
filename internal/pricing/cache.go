package pricing

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

type entry struct {
	Sheet    json.RawMessage `json:"sheet,omitempty"`
	Stamp    string          `json:"stamp,omitempty"` // json layer: the file's size and mtime the sheet was made from
	Fetched  time.Time       `json:"fetched,omitzero"`
	Attempt  time.Time       `json:"attempt,omitzero"`
	Error    string          `json:"error,omitempty"`
	ETag     string          `json:"etag,omitempty"`
	Modified string          `json:"modified,omitempty"`
}

type cache struct {
	Layers map[string]*entry `json:"layers"`
}

func (c cache) write(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, cacheFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, cacheFile))
}

func readCache(dir string) cache {
	c := cache{Layers: map[string]*entry{}}
	if data, err := os.ReadFile(filepath.Join(dir, cacheFile)); err == nil {
		json.Unmarshal(data, &c) // unreadable: nothing cached
	}
	if c.Layers == nil {
		c.Layers = map[string]*entry{}
	}
	return c
}

// due reports whether a cached layer should be refreshed now by itself.
func (c Config) due(name string, cached *entry) bool {
	if name == LayerJSON {
		// A changed file is news even in manual mode.
		stamp, err := fileStamp(c.File)
		if err == nil && (cached == nil || cached.Stamp != stamp) {
			return cached == nil || cached.Error == "" || c.now().Sub(cached.Attempt) >= FailureBackoff
		}
	}
	if c.Manual {
		return false
	}
	if cached == nil || cached.Fetched.IsZero() && cached.Attempt.IsZero() {
		return true
	}
	if cached.Error != "" && c.now().Sub(cached.Attempt) < FailureBackoff {
		return false
	}
	return c.now().Sub(cached.Fetched) >= c.ttl()
}

// fromCache turns a layer's cached sheet into prices and fills its status. No sheet yet is not
// an error unless the last run failed.
func fromCache(cached *entry, name string, layer *Layer, cfg Config) ([]cost.Override, error) {
	if cached == nil {
		return nil, nil
	}
	layer.Fetched = cached.Fetched
	switch {
	case cfg.Manual:
	case !cached.Fetched.IsZero():
		layer.Next = cached.Fetched.Add(cfg.ttl())
	case cached.Error != "": // nothing fetched yet: the failure's backoff dates the next try
		layer.Next = cached.Attempt.Add(FailureBackoff)
	}
	var err error
	if cached.Error != "" {
		err = errors.New(cached.Error)
	}
	if len(cached.Sheet) == 0 {
		return nil, err
	}
	list, perr := cost.ParsePrices(cached.Sheet, name)
	if perr != nil {
		return nil, perr
	}
	if err != nil { // last good prices stay in force; the status shows why they are old
		layer.Error = err.Error()
		layer.Models = len(list)
		return list, nil
	}
	return list, nil
}
