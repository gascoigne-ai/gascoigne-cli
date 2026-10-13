package prose

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

const ortVersion = "1.29.0"

// runtimes are the ONNX Runtime release archives per platform and variant,
// with the hashes GitHub publishes for them: a download that does not match is
// refused. macOS needs no GPU variant: CoreML is built in.
var runtimes = map[string]struct{ file, sha256 string }{
	"linux/amd64/cpu":      {"onnxruntime-linux-x64-" + ortVersion + ".tgz", "c3fddc4f139a045b0c4902c57410f0694f1c2fdf9b6939fbe38b1aeae7cd14ba"},
	"linux/amd64/cuda12":   {"onnxruntime-linux-x64-gpu_cuda12-" + ortVersion + ".tgz", "4ca594a0da83927befbd73fe020d7f569be151d70bb4fe9741ad405f4882e2ad"},
	"linux/amd64/cuda13":   {"onnxruntime-linux-x64-gpu_cuda13-" + ortVersion + ".tgz", "844c64acfc43ab9423215c26493055ea229268e28283146cc644ecef0bdae048"},
	"linux/arm64/cpu":      {"onnxruntime-linux-aarch64-" + ortVersion + ".tgz", "e1799098ebc054b370f6176a450f158720f297818c613e5dc99b92e2ec82346f"},
	"darwin/arm64/cpu":     {"onnxruntime-osx-arm64-" + ortVersion + ".tgz", "d0706fc34f315d8c88639d0a8c81f2e09e815f282cabed3493c06a054352cf92"},
	"windows/amd64/cpu":    {"onnxruntime-win-x64-" + ortVersion + ".zip", "c9b4b7086b529ad814f428c1bad028e20a25d7dc0699836775faace4ab5b78b2"},
	"windows/amd64/cuda12": {"onnxruntime-win-x64-gpu_cuda12-" + ortVersion + ".zip", "3526773ba1fef5849a3e9d18558d87bbbb75b0db3ddcde4f08694a746ac3169f"},
	"windows/amd64/cuda13": {"onnxruntime-win-x64-gpu_cuda13-" + ortVersion + ".zip", "0d2ab3feb9ed0ae68ccbbf2013ac87ad58a3b31bc8b28c5854f2ccacecbd8d90"},
	"windows/arm64/cpu":    {"onnxruntime-win-arm64-" + ortVersion + ".zip", "a094a49c3ced0f9fca554647cc7566ae99d93a63a8ce6bf47975561c2de7608e"},
}

const releaseURL = "https://github.com/microsoft/onnxruntime/releases/download/v" + ortVersion + "/"

// libName is the runtime's main library on this OS.
func libName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libonnxruntime.dylib"
	case "windows":
		return "onnxruntime.dll"
	}
	return "libonnxruntime.so"
}

var cudaVersion = regexp.MustCompile(`CUDA Version: (\d+)`)

// pickVariant resolves "auto" from the driver's CUDA version (nvidia-smi):
// the release builds target CUDA 12 and 13.
func pickVariant(variant string) string {
	if variant != "auto" {
		return variant
	}
	out, err := exec.Command("nvidia-smi").Output()
	if m := cudaVersion.FindSubmatch(out); err == nil && m != nil {
		switch string(m[1]) {
		case "13":
			return "cuda13"
		case "12":
			return "cuda12"
		}
	}
	return "cpu"
}

// installRuntime downloads the ONNX Runtime libraries into dir (the CPU
// build is 10 MB, the CUDA ones 190-420 MB) unless that archive is there.
func installRuntime(ctx context.Context, dir, variant string) error {
	variant = pickVariant(variant)
	asset, ok := runtimes[runtime.GOOS+"/"+runtime.GOARCH+"/"+variant]
	if !ok {
		return fmt.Errorf("no ONNX Runtime %s build for %s/%s: set -lib to your own", variant, runtime.GOOS, runtime.GOARCH)
	}
	marker := filepath.Join(dir, ".archive")
	fmt.Fprintf(os.Stderr, "gascoigne prose: %s\n  sha256 %s\n", asset.file, asset.sha256)
	// The marker alone is not enough: the library may have been deleted since.
	if got, _ := os.ReadFile(marker); string(got) == asset.sha256 && fileExists(filepath.Join(dir, libName())) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	archive := filepath.Join(dir, asset.file)
	defer os.Remove(archive)
	fmt.Fprintf(os.Stderr, "gascoigne prose: downloading %s\n", asset.file)
	if err := fetch(ctx, releaseURL+asset.file, archive); err != nil {
		return fmt.Errorf("download %s: %w", asset.file, err)
	}
	if err := verify(archive, asset.sha256); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "  downloaded, checksum matches")
	if err := extractLibs(archive, dir); err != nil {
		return fmt.Errorf("unpack %s: %w", asset.file, err)
	}
	if strings.HasPrefix(variant, "cuda") {
		fmt.Fprintln(os.Stderr, "gascoigne prose: the GPU build also needs the CUDA runtime and cuDNN 9 on the library path")
	}
	return os.WriteFile(marker, []byte(asset.sha256), 0o644)
}

// extractLibs flattens the archive's lib/ files into dir, where the runtime
// finds its provider libraries next to the main one.
func extractLibs(archive, dir string) error {
	if strings.HasSuffix(archive, ".zip") {
		return extractZip(archive, dir)
	}
	f, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !isLib(h.Name) {
			continue
		}
		dst := filepath.Join(dir, filepath.Base(h.Name))
		switch h.Typeflag {
		case tar.TypeReg:
			err = write(dst, tr, h.FileInfo().Mode())
		case tar.TypeSymlink:
			os.Remove(dst)
			err = os.Symlink(filepath.Base(h.Linkname), dst)
		}
		if err != nil {
			return err
		}
	}
}

func extractZip(archive, dir string) error {
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.FileInfo().IsDir() || !isLib(f.Name) {
			continue
		}
		dst, ok := libPath(dir, f.Name)
		if !ok {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return err
		}
		err = write(dst, r, f.Mode())
		r.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// libPath is where an archive entry lands: its base name directly in dir.
// Names that would leave dir are refused.
func libPath(dir, name string) (string, bool) {
	dst := filepath.Join(dir, filepath.Base(name))
	return dst, strings.HasPrefix(dst, filepath.Clean(dir)+string(os.PathSeparator))
}

// isLib selects the shared libraries directly in the archive's lib/ directory:
// not its cmake and pkg-config files, nor the debug bundle that holds a file
// named like the real library.
func isLib(name string) bool {
	if filepath.Base(filepath.Dir(name)) != "lib" {
		return false
	}
	base := filepath.Base(name)
	return strings.Contains(base, ".so") || strings.HasSuffix(base, ".dylib") || strings.HasSuffix(base, ".dll")
}

func write(path string, r io.Reader, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode|0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}
