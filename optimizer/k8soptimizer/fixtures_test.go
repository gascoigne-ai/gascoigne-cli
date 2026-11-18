package k8soptimizer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress/table"
)

// Real kubectl output from testdata/eval: a List of pods is sampled to
// MaxItems, unhealthy pods first. The golden file pins the output byte for
// byte.
func TestManifestsSamplesPodList(t *testing.T) {
	input, err := os.ReadFile(filepath.Join("..", "..", "testdata", "eval", "k8s", "pods.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "pods-sampled.golden"))
	if err != nil {
		t.Fatal(err)
	}
	o := &Optimizer{cfg: &Config{MaxItems: 3, Limits: table.Limits{MaxRows: 30, MaxCell: 60}, NotesLines: 5}}
	got, err := o.manifests(string(input))
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
