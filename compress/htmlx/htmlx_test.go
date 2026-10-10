package htmlx

import (
	"strings"
	"testing"
)

func TestHeadingWithWrappedText(t *testing.T) {
	page := `<html><body><main><h2 id="hook-lifecycle"><div><a href="#hook-lifecycle">` + "​" + `</a></div><span>Hook lifecycle</span></h2><p>Runs at points.</p></main></body></html>`
	got, err := Markdown(page, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "## Hook lifecycle\n\nRuns at points.") {
		t.Fatalf("got %q", got)
	}
}
