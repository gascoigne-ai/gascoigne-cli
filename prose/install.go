package prose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// runtimeDir holds the ONNX Runtime libraries inside the model directory.
const runtimeDir = "onnxruntime"

const revision = "e00cc5e60ff73b01674ef8fa6da6c5acf6d54181"

// files are the model's artifacts at the pinned revision: a download that
// does not match its hash is refused.
var files = []struct{ remote, local, sha256 string }{
	{"tokenizer.json", "tokenizer.json", "6c8aaa9a542084f2457eab775d4eeb51f92a70c0fd9de28d5edb0ddec3c08d30"},
	{"onnx/kompress-fp32.onnx", "model.onnx", "0f6758d903b5b5b90f784b7f17b599277f040a3c268afb88c193fe5bb9c08649"},
}

// Install downloads what is missing into dir: the ONNX Runtime libraries
// (in dir/onnxruntime) and the model (601 MB for the graph).
func Install(ctx context.Context, dir, variant string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := installRuntime(ctx, filepath.Join(dir, runtimeDir), variant); err != nil {
		return err
	}
	for _, f := range files {
		path := filepath.Join(dir, f.local)
		fmt.Fprintf(os.Stderr, "gascoigne prose: %s\n  sha256 %s\n", f.remote, f.sha256)
		if verify(path, f.sha256) == nil {
			continue
		}
		url := "https://huggingface.co/chopratejas/kompress-v2-base/resolve/" + revision + "/" + f.remote
		if err := fetch(ctx, url, path); err != nil {
			return fmt.Errorf("download %s: %w", f.remote, err)
		}
		if err := verify(path, f.sha256); err != nil {
			os.Remove(path)
			return err
		}
		fmt.Fprintln(os.Stderr, "  downloaded, checksum matches")
	}
	return nil
}

func fetch(ctx context.Context, url, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", res.Status)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, res.Body)
	return err
}

func verify(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("%s: sha256 %s, want %s", path, got, want)
	}
	return nil
}
