package dataoptimizer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gascoigne-ai/gascoigne-cli/compress/jsonx"
)

func testOptimizer() *Optimizer {
	return &Optimizer{cfg: &Config{JSON: *jsonx.Default(), MaxRows: 30, MaxCell: 60, MaxItems: 6}}
}

func TestTOMLFoldsLockEntries(t *testing.T) {
	in := `version = 1

[[package]]
name = "anyio"
version = "4.6.0"
source = { registry = "https://pypi.org/simple" }
dependencies = [
    { name = "idna" },
    { name = "sniffio", marker = "python_version < '3.11'" },
]
sdist = { url = "https://files/anyio.tar.gz", hash = "sha256:abc" }

[[package]]
name = "idna"
version = "3.10"

[[package]]
name = "serde"
version = "1.0.1"
dependencies = [
 "serde_derive 1.0.1",
]
`
	got := testOptimizer().toml(in)
	want := "version = 1\n\n[[package]] ×3\n  anyio 4.6.0 deps: idna, sniffio\n  idna 3.10\n  serde 1.0.1 deps: serde_derive\n"
	if got != want {
		t.Fatalf("got:\n%s", got)
	}
}

func TestXMLKeepsFailures(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><!-- generated --><testsuite xmlns:xsi="x" tests="20">`)
	for i := range 20 {
		if i == 10 {
			fmt.Fprintf(&b, `<testcase name="t%d"><failure message="boom">trace</failure></testcase>`, i)
			continue
		}
		fmt.Fprintf(&b, `<testcase name="t%d"/>`, i)
	}
	b.WriteString(`</testsuite>`)
	got, err := testOptimizer().xml(b.String())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<testsuite tests="20">`, `<testcase name="t3"/>`, "<!-- … 6 <testcase> elements omitted -->",
		`<failure message="boom">trace</failure>`, "<!-- … 7 <testcase> elements omitted -->", `<testcase name="t19"/>`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "generated") || strings.Contains(got, "xmlns") || strings.Contains(got, `"t5"`) {
		t.Errorf("kept noise:\n%s", got)
	}
}

func TestCSVAsTable(t *testing.T) {
	got := testOptimizer().csv("id\tcountry\tstatus\n1\tPL\tpaid\n2\tPL\tfailed\n3\tPL\tpaid\n", nil)
	if !strings.Contains(got, "country=PL") || !strings.Contains(got, "failed") {
		t.Fatalf("got:\n%s", got)
	}
}

func TestYAMLKeepsSmallerOriginal(t *testing.T) {
	in := "a: 1\nb: 2\n"
	if got, err := testOptimizer().yaml(in, nil); err != nil || got != in {
		t.Fatalf("got %q, %v", got, err)
	}
}
