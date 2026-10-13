package prose

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestInstalled(t *testing.T) {
	dir := t.TempDir()
	if model, rt := Installed(dir); model || rt {
		t.Fatal("empty dir is not installed")
	}
	put := func(rel string) {
		path := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put("tokenizer.json")
	if model, _ := Installed(dir); model {
		t.Fatal("tokenizer alone is not the model")
	}
	put("model.onnx")
	put(filepath.Join(runtimeDir, libName()))
	if model, rt := Installed(dir); !model || !rt {
		t.Fatalf("model %v, runtime %v", model, rt)
	}
}

func TestVerify(t *testing.T) {
	sum := sha256.Sum256([]byte("model"))
	defer func(old []struct{ remote, local, sha256 string }) { files = old }(files)
	files = []struct{ remote, local, sha256 string }{{"m", "model.onnx", hex.EncodeToString(sum[:])}}

	dir := t.TempDir()
	put := func(rel, text string) {
		path := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if Verify(dir) == nil {
		t.Fatal("empty dir verifies")
	}
	put(filepath.Join(runtimeDir, libName()), "")
	if Verify(dir) == nil {
		t.Fatal("missing model verifies")
	}
	put("model.onnx", "damaged")
	if Verify(dir) == nil {
		t.Fatal("wrong checksum verifies")
	}
	put("model.onnx", "model")
	if err := Verify(dir); err != nil {
		t.Fatal(err)
	}
}
