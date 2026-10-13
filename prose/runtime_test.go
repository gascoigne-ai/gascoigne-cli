package prose

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractLibsTar(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "rt.tgz")
	f, _ := os.Create(archive)
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	add := func(h tar.Header, body string) {
		h.Size = int64(len(body))
		tw.WriteHeader(&h)
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
	if got := pickVariant("cuda12"); got != "cuda12" {
		t.Fatalf("got %q", got)
	}
}
