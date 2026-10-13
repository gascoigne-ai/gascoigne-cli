// Package prose runs the prose compression model (a ModernBERT extractive
// compressor, ONNX) on the GPU when there is one: CUDA on Linux and Windows,
// CoreML (Metal, Neural Engine) on macOS, else the CPU. `gascoigne prose
// serve` speaks the line protocol of package serve; the doc optimizer starts
// it. Built without cgo: the ONNX Runtime library is opened at run time
// (package ortrt) and fetched by Install.
package prose

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/gascoigne-ai/gascoigne-cli/prose/bpe"
	"github.com/gascoigne-ai/gascoigne-cli/prose/serve"
)

// ModelDir is where Install puts the model and the ONNX Runtime libraries.
func ModelDir() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "gascoigne", "prose-model"), nil
}

// Serve answers requests from in on out until in closes. lib is the ONNX
// Runtime library ("" finds the one Install put in dir, else the system's);
// device is auto, cuda, coreml or cpu.
func Serve(in io.Reader, out io.Writer, dir, lib, device string) error {
	tok, err := bpe.Load(filepath.Join(dir, "tokenizer.json"))
	if err != nil {
		return err
	}
	if lib == "" {
		if own := filepath.Join(dir, runtimeDir, libName()); fileExists(own) {
			lib = own
		}
	}
	sc, err := newORT(filepath.Join(dir, "model.onnx"), lib, device)
	if err != nil {
		return err
	}
	defer sc.close()
	fmt.Fprintf(os.Stderr, "gascoigne prose: device %s\n", sc.device)
	return serve.Run(in, out, tok, sc, sc.device)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Installed reports whether Install's files are in dir: the model with its
// tokenizer, and the ONNX Runtime library (a system-wide one is not looked for).
func Installed(dir string) (model, runtime bool) {
	model = fileExists(filepath.Join(dir, "model.onnx")) && fileExists(filepath.Join(dir, "tokenizer.json"))
	return model, fileExists(filepath.Join(dir, runtimeDir, libName()))
}
