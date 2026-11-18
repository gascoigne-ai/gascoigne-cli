package pricing

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/gascoigne-ai/gascoigne-cli/internal/cost"
)

// Load builds the user's prices from the settings and the cache. It reads files and never the
// network or the script; Stale says a Refresh is due.
func Load(cfg Config) Result {
	load := &loader{cfg: cfg, cached: readCache(cfg.StateDir)}
	load.static()
	load.file()
	load.remote()
	load.mergeLists()
	return load.result
}

// loader collects the layers of one Load: each layer's status goes to result,
// its prices to lists, in precedence order.
type loader struct {
	cfg    Config
	cached cache
	result Result
	lists  [][]cost.Override
}

func (load *loader) add(layer Layer, list []cost.Override, err error) {
	if err != nil {
		layer.Error = err.Error()
	} else {
		layer.Models = len(list)
		load.lists = append(load.lists, list)
	}
	load.result.Layers = append(load.result.Layers, layer)
}

// static adds the prices set in the config itself.
func (load *loader) static() {
	if len(load.cfg.Static) == 0 {
		return
	}
	sheet, err := cost.Sheet(load.cfg.Static)
	var list []cost.Override
	if err == nil {
		list, err = cost.ParsePrices(sheet, LayerStatic)
	}
	load.add(Layer{Name: LayerStatic}, list, err)
}

// file adds the price sheet file: read directly, or through the cache when a
// translate command turns it into a sheet.
func (load *loader) file() {
	if load.cfg.File == "" {
		return
	}
	layer := Layer{Name: LayerJSON}
	var list []cost.Override
	var err error
	if load.cfg.Translate == "" {
		var data []byte
		if data, err = readSheet(load.cfg.File); err == nil {
			list, err = cost.ParsePrices(data, LayerJSON)
		}
	} else {
		cached := load.cached.Layers[LayerJSON]
		list, err = fromCache(cached, LayerJSON, &layer, load.cfg)
		if load.cfg.due(LayerJSON, cached) {
			load.result.Stale = true
		}
	}
	load.add(layer, list, err)
}

// remote adds the layers fetched from the network or a script, from the cache.
func (load *loader) remote() {
	for _, name := range []string{LayerAPI, LayerScript} {
		if !slices.Contains(load.cfg.remote(), name) {
			continue
		}
		layer := Layer{Name: name}
		cached := load.cached.Layers[name]
		list, err := fromCache(cached, name, &layer, load.cfg)
		if load.cfg.due(name, cached) {
			load.result.Stale = true
		}
		load.add(layer, list, err)
	}
}

// mergeLists keeps the first list that names a model.
func (load *loader) mergeLists() {
	seen := map[string]bool{}
	for _, list := range load.lists {
		for _, override := range list {
			if !seen[override.Model.ID] {
				seen[override.Model.ID] = true
				load.result.Overrides = append(load.result.Overrides, override)
			}
		}
	}
}

// readSheet reads a price sheet file: a regular file of at most MaxBytes. A directory, a device or
// a FIFO (which never ends) and an oversized file are refused before they are read.
func readSheet(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if info.Size() > MaxBytes {
		return nil, fmt.Errorf("%s is over %d bytes", path, MaxBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBytes {
		return nil, fmt.Errorf("%s is over %d bytes", path, MaxBytes)
	}
	return data, nil
}

// fileStamp identifies a version of the file.
func fileStamp(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%d-%d", info.Size(), info.ModTime().UnixNano()), nil
}
