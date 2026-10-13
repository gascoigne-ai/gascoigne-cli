package prose

import (
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
