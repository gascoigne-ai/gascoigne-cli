// Package prose runs the prose compression model (a ModernBERT extractive
// compressor, ONNX) on the GPU when there is one: CUDA on Linux and Windows,
// CoreML (Metal, Neural Engine) on macOS, else the CPU. It runs inside the
// process that optimizes (the background server keeps it loaded). Built without cgo: the ONNX Runtime library is opened at run time
// (package ortrt) and fetched by Install.
package prose

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/gascoigne-ai/gascoigne-cli/internal/prose/bpe"
	"github.com/gascoigne-ai/gascoigne-cli/internal/prose/keep"
)

// ModelDir is the root where Install puts the models and the ONNX Runtime libraries.
func ModelDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "gascoigne", "prose-model"), nil
}

// Model is the loaded tokenizer and ONNX session. Calls are serialized: the
// session is one set of buffers on one device.
type Model struct {
	mu     sync.Mutex
	tok    *bpe.Tokenizer
	sc     *ortScorer
	device string
}

// Open loads the named model Install put under root. lib is the ONNX Runtime
// library ("" finds the one Install put in root, else the system's); device is
// auto, cuda, coreml or cpu.
func Open(root, lib, device, name string) (*Model, error) {
	s, dir, err := lookup(root, name)
	if err != nil {
		return nil, err
	}
	tok, err := bpe.Load(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		return nil, err
	}
	if lib == "" {
		if own := filepath.Join(root, runtimeDir, libName()); fileExists(own) {
			lib = own
		}
	}
	sc, err := newORT(filepath.Join(dir, "model.onnx"), lib, device, s.out)
	if err != nil {
		return nil, err
	}
	return &Model{tok: tok, sc: sc, device: sc.device}, nil
}

// Device is the one the model runs on: cuda, coreml or cpu.
func (m *Model) Device() string { return m.device }

// Keep marks the words that have a token scoring at least threshold.
func (m *Model) Keep(words []string, threshold float64) ([]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return keep.Words(words, threshold, m.tok, m.sc)
}

func (m *Model) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sc.close()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Installed reports whether Install's files are under root: the named model
// with its tokenizer, and the ONNX Runtime library (a system-wide one is not
// looked for).
func Installed(root, name string) (model, runtime bool) {
	_, dir, err := lookup(root, name)
	if err != nil {
		return false, false
	}
	model = fileExists(filepath.Join(dir, "model.onnx")) && fileExists(filepath.Join(dir, "tokenizer.json"))
	return model, fileExists(filepath.Join(root, runtimeDir, libName()))
}
