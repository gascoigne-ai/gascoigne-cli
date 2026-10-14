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
	if model, rt := Installed(dir, Kompress); model || rt {
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
	if model, _ := Installed(dir, Kompress); model {
		t.Fatal("tokenizer alone is not the model")
	}
	put("model.onnx")
	put(filepath.Join(runtimeDir, libName()))
	if model, rt := Installed(dir, Kompress); !model || !rt {
		t.Fatalf("model %v, runtime %v", model, rt)
	}
}

func TestVerify(t *testing.T) {
	sum := sha256.Sum256([]byte("model"))
	defer func(old map[string]spec) { models = old }(models)
	models = map[string]spec{Kompress: {files: []file{{"m", "model.onnx", hex.EncodeToString(sum[:])}}}}

	dir := t.TempDir()
	put := func(rel, text string) {
		path := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if Verify(dir, Kompress) == nil {
		t.Fatal("empty dir verifies")
	}
	put(filepath.Join(runtimeDir, libName()), "")
	if Verify(dir, Kompress) == nil {
		t.Fatal("missing model verifies")
	}
	put("model.onnx", "damaged")
	if Verify(dir, Kompress) == nil {
		t.Fatal("wrong checksum verifies")
	}
	put("model.onnx", "model")
	if err := Verify(dir, Kompress); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownModel(t *testing.T) {
	if Verify(t.TempDir(), "nope") == nil {
		t.Fatal("unknown model verifies")
	}
	if model, rt := Installed(t.TempDir(), "nope"); model || rt {
		t.Fatal("unknown model is installed")
	}
}

// The ctxprune model lives in its own directory and shares the runtime.
func TestCtxpruneModelLayout(t *testing.T) {
	root := t.TempDir()
	put := func(rel string) {
		path := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	put(filepath.Join(runtimeDir, libName()))
	put("tokenizer.json")
	put("model.onnx")
	if model, _ := Installed(root, Ctxprune); model {
		t.Fatal("the kompress files are not the ctxprune model")
	}
	put("ctxprune-small/tokenizer.json")
	put("ctxprune-small/model.onnx")
	if model, rt := Installed(root, Ctxprune); !model || !rt {
		t.Fatalf("model %v, runtime %v", model, rt)
	}
}

func TestKeepProb(t *testing.T) {
	got := keepProb([]float32{0, 0, -20, 20, 20, -20})
	for i, want := range []float32{0.5, 1, 0} {
		if d := got[i] - want; d > 1e-6 || d < -1e-6 {
			t.Errorf("token %d: %v, want %v", i, got[i], want)
		}
	}
}
