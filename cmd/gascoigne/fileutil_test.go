package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveJSON(t *testing.T) {
	sandbox(t)
	file := filepath.Join(t.TempDir(), "state", "x.json")
	if err := saveJSON(file, map[string]int{"a": 1}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) != "{\n  \"a\": 1\n}" {
		t.Fatalf("content %q, err %v", data, err)
	}
	info, _ := os.Stat(file)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file perm %v, want 0600", info.Mode().Perm())
	}
	dirInfo, _ := os.Stat(filepath.Dir(file))
	if dirInfo.Mode().Perm() != 0o700 {
		t.Errorf("dir perm %v, want 0700", dirInfo.Mode().Perm())
	}
	if _, err := os.Stat(file + ".tmp"); err == nil {
		t.Error("temp file left behind")
	}
}

func TestSaveJSONMarshalError(t *testing.T) {
	sandbox(t)
	file := filepath.Join(t.TempDir(), "x.json")
	write(t, file, "old")
	if err := saveJSON(file, make(chan int)); err == nil {
		t.Fatal("want marshal error")
	}
	if data, _ := os.ReadFile(file); string(data) != "old" {
		t.Errorf("file changed to %q on marshal error", data)
	}
}

func TestWriteWithBackup(t *testing.T) {
	sandbox(t)
	tests := []struct {
		name   string
		suffix string
		perm   os.FileMode
	}{
		{"user suffix", ".gascoigne-bak", 0o600},
		{"short suffix", ".bak", 0o644},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "conf")
			write(t, file, "old")
			backup, err := writeWithBackup(file, test.suffix, []byte("old"), []byte("new"), test.perm)
			if err != nil {
				t.Fatal(err)
			}
			if backup != file+test.suffix {
				t.Errorf("backup %q, want %q", backup, file+test.suffix)
			}
			if data, _ := os.ReadFile(backup); string(data) != "old" {
				t.Errorf("backup holds %q", data)
			}
			if data, _ := os.ReadFile(file); string(data) != "new" {
				t.Errorf("file holds %q", data)
			}
			info, _ := os.Stat(backup)
			if info.Mode().Perm() != test.perm {
				t.Errorf("backup perm %v, want %v", info.Mode().Perm(), test.perm)
			}
		})
	}
}

func TestEncodeJSONNoEscape(t *testing.T) {
	data, err := encodeJSON(map[string]string{"k": "<a&b>"})
	if err != nil || string(data) != "{\"k\":\"<a&b>\"}\n" {
		t.Fatalf("got %q, err %v", data, err)
	}
	if _, err := encodeJSON(make(chan int)); err == nil {
		t.Error("want error for channel")
	}
}

func TestReadJSONFile(t *testing.T) {
	sandbox(t)
	good, bad := filepath.Join(t.TempDir(), "good.json"), filepath.Join(t.TempDir(), "bad.json")
	write(t, good, `{"a":1}`)
	write(t, bad, `{`)
	var out map[string]any
	if !readJSONFile(good, &out) || out["a"] != 1.0 {
		t.Errorf("good file: %v", out)
	}
	if readJSONFile(bad, &out) || readJSONFile(filepath.Join(t.TempDir(), "none"), &out) {
		t.Error("bad or missing file must report false")
	}
	if readJSON(bad) != nil || readJSON(good)["a"] != 1.0 {
		t.Error("readJSON must derive from readJSONFile")
	}
}
