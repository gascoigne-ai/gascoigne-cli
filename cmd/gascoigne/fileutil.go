package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// saveJSON writes value as indented JSON to file atomically (temp file, then
// rename): readers such as hooks run while commands write. The state
// directory is created private. A value that cannot be encoded leaves file
// untouched.
func saveJSON(file string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(file), err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}

// writeWithBackup keeps old next to file as file+suffix, then writes updated
// to file; both get perm. It returns the backup path. The suffix is
// user-visible, so each caller passes its own.
func writeWithBackup(file, suffix string, old, updated []byte, perm os.FileMode) (string, error) {
	backup := file + suffix
	if err := os.WriteFile(backup, old, perm); err != nil {
		return "", fmt.Errorf("back up %s: %w", file, err)
	}
	if err := os.WriteFile(file, updated, perm); err != nil {
		return backup, fmt.Errorf("write %s: %w", file, err)
	}
	return backup, nil
}

// encodeJSON is json.Marshal without HTML escaping (<, >, & stay as written),
// ending in a newline like json.Encoder.
func encodeJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
