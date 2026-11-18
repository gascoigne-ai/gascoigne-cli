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
	"time"

	"github.com/gascoigne-ai/gascoigne-cli/internal/httpget"
)

// runtimeDir holds the ONNX Runtime libraries inside the model directory.
const runtimeDir = "onnxruntime"

// Verify checks what Install leaves in root: the runtime library and each file
// of the named model against its pinned hash. The error names the first thing
// wrong; Install fetches again whatever fails here.
func Verify(root, name string) error {
	s, dir, err := lookup(root, name)
	if err != nil {
		return err
	}
	if _, rt := Installed(root, name); !rt {
		return fmt.Errorf("ONNX Runtime library missing in %s", filepath.Join(root, runtimeDir))
	}
	for _, f := range s.files {
		if err := verify(filepath.Join(dir, f.local), f.sha256); err != nil {
			return err
		}
	}
	return nil
}

// Install downloads what is missing into root: the ONNX Runtime libraries
// (in root/onnxruntime, shared by the models) and the named model (default:
// 274 MB, int8 weight-only graph; agent: 563 MB).
func Install(ctx context.Context, root, variant, name string) error {
	return install(ctx, os.Stderr, root, variant, name)
}

// install is Install with its progress lines written to out.
func install(ctx context.Context, out io.Writer, root, variant, name string) error {
	s, dir, err := lookup(root, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := installRuntime(ctx, out, filepath.Join(root, runtimeDir), variant); err != nil {
		return err
	}
	for _, f := range s.files {
		path := filepath.Join(dir, f.local)
		fmt.Fprintf(out, "gascoigne prose: %s\n  sha256 %s\n", f.remote, f.sha256)
		if verify(path, f.sha256) == nil {
			continue
		}
		url := "https://huggingface.co/" + s.repo + "/resolve/" + s.revision + "/" + f.remote
		if err := fetch(ctx, url, path); err != nil {
			return fmt.Errorf("download %s: %w", f.remote, err)
		}
		if err := verify(path, f.sha256); err != nil {
			os.Remove(path)
			return err
		}
		fmt.Fprintln(out, "  downloaded, checksum matches")
	}
	return nil
}

// headerTimeout bounds the wait for a server to start answering; the body
// itself is bounded by ctx only, as the archives are large.
var headerTimeout = 30 * time.Second

// newClient is an HTTP client that gives up on a server that never answers.
func newClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = headerTimeout
	return &http.Client{Transport: t}
}

func fetch(ctx context.Context, url, path string) error {
	res, err := httpget.Do(ctx, newClient(), url, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", res.Status)
	}
	// Written aside and renamed, so a cut-off download never looks installed.
	part := path + ".part"
	f, err := os.Create(part)
	if err != nil {
		return err
	}
	_, err = copyWithBar(f, res.Body, res.ContentLength)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(part)
		return err
	}
	return os.Rename(part, path)
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
