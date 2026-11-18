package prose

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestExtractLibsTar(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "rt.tgz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(header tar.Header, body string) {
		header.Size = int64(len(body))
		tw.WriteHeader(&header)
		tw.Write([]byte(body))
	}
	add(tar.Header{Name: "ort-1/lib/libonnxruntime.so.1.29.0", Mode: 0o755, Typeflag: tar.TypeReg}, "lib")
	add(tar.Header{Name: "ort-1/lib/libonnxruntime.so", Linkname: "libonnxruntime.so.1.29.0", Typeflag: tar.TypeSymlink}, "")
	add(tar.Header{Name: "ort-1/include/onnxruntime_c_api.h", Mode: 0o644, Typeflag: tar.TypeReg}, "header")
	add(tar.Header{Name: "ort-1/lib/libonnxruntime.1.29.0.dylib.dSYM/Contents/Resources/DWARF/libonnxruntime.1.29.0.dylib", Mode: 0o644, Typeflag: tar.TypeReg}, "debug")
	add(tar.Header{Name: "ort-1/lib/../../escape", Mode: 0o644, Typeflag: tar.TypeReg}, "x")
	tw.Close()
	gz.Close()
	f.Close()

	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	if err := extractLibs(archive, out); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(out, "libonnxruntime.so")); err != nil || string(b) != "lib" {
		t.Fatalf("symlinked lib: %q, %v", b, err)
	}
	if b, _ := os.ReadFile(filepath.Join(out, "libonnxruntime.1.29.0.dylib")); string(b) == "debug" {
		t.Error("debug bundle file overwrote the library name")
	}
	for _, p := range []string{filepath.Join(out, "onnxruntime_c_api.h"), filepath.Join(dir, "escape")} {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("%s must not be extracted", p)
		}
	}
}

func TestExtractLibsZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "rt.zip")
	f, _ := os.Create(archive)
	zw := zip.NewWriter(f)
	for name, body := range map[string]string{"ort-1/lib/onnxruntime.dll": "dll", "ort-1/README.md": "doc"} {
		w, _ := zw.Create(name)
		w.Write([]byte(body))
	}
	zw.Close()
	f.Close()
	if err := extractLibs(archive, dir); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "onnxruntime.dll")); err != nil || string(b) != "dll" {
		t.Fatalf("got %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "README.md")); err == nil {
		t.Error("files outside lib/ must not be extracted")
	}
}

func TestPickVariantExplicit(t *testing.T) {
	if got := pickVariant(context.Background(), "cuda12"); got != "cuda12" {
		t.Fatalf("got %q", got)
	}
}

func TestEntryName(t *testing.T) {
	for entry, want := range map[string]string{
		"onnx/lib/libonnxruntime.so":   "libonnxruntime.so",
		"lib/../lib/evil.so":           "evil.so", // cleaned: still directly in lib/
		"onnx/lib/../../escape.so":     "",
		"../../etc/passwd":             "",
		"onnx/lib/..":                  "",
		"onnx/include/onnxruntime.dll": "",
		"":                             "",
	} {
		got, ok := entryName(entry)
		if got != want || ok != (want != "") {
			t.Errorf("entryName(%q) = %q, %v; want %q", entry, got, ok, want)
		}
	}
}

func TestExtractLibsKeepsSymlinksInside(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "rt.tgz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "ort/lib/libevil.so", Linkname: "../../outside", Typeflag: tar.TypeSymlink})
	tw.Close()
	gz.Close()
	f.Close()

	out := filepath.Join(dir, "out")
	os.Mkdir(out, 0o755)
	if err := extractLibs(archive, out); err != nil {
		t.Fatal(err)
	}
	if target, err := os.Readlink(filepath.Join(out, "libevil.so")); err != nil || target != "outside" {
		t.Fatalf("link target %q, %v; want the bare name", target, err)
	}
}

func TestPickVariantHungSMI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell stub")
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "nvidia-smi"), []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	old := smiTimeout
	smiTimeout = 100 * time.Millisecond
	defer func() { smiTimeout = old }()
	start := time.Now()
	if got := pickVariant(context.Background(), "auto"); got != "cpu" {
		t.Fatalf("got %q", got)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("hung nvidia-smi not cut off")
	}
}

func TestFetchHungServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	old := headerTimeout
	headerTimeout = 100 * time.Millisecond
	defer func() { headerTimeout = old }()
	start := time.Now()
	if err := fetch(context.Background(), srv.URL, filepath.Join(t.TempDir(), "f")); err == nil {
		t.Fatal("want error")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("hung server not cut off")
	}
}

func TestInstallRuntimeWritesToWriter(t *testing.T) {
	asset, ok := runtimes[runtime.GOOS+"/"+runtime.GOARCH+"/cpu"]
	if !ok {
		t.Skip("no cpu build for this platform")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".archive"), []byte(asset.sha256), 0o644)
	os.WriteFile(filepath.Join(dir, libName()), nil, 0o644)
	var buf bytes.Buffer
	if err := installRuntime(context.Background(), &buf, dir, "cpu"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), asset.sha256) {
		t.Fatalf("got %q", buf.String())
	}
}
