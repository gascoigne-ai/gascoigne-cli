package testreport

import (
	"bufio"
	"encoding/json"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

type event struct {
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

// GoJSON compresses go test -json events: they are folded back into plain
// go test output (failed tests and package results only), then into Go.
func GoJSON(text string) ([]string, error) {
	plain, err := jsonToText(text)
	if err != nil {
		return nil, err
	}
	return Go(compress.Lines(plain)), nil
}

// pkgEvents collects what go test -json said about one package.
type pkgEvents struct {
	out    map[string][]string // test → output
	failed []string
	result string
}

// add folds one event into the package.
func (p *pkgEvents) add(ev event) {
	switch ev.Action {
	case "output":
		p.out[ev.Test] = append(p.out[ev.Test], strings.TrimSuffix(ev.Output, "\n"))
	case "fail":
		if ev.Test != "" {
			p.failed = append(p.failed, ev.Test)
		} else {
			p.result = "FAIL\t" + ev.Package
		}
	case "pass":
		if ev.Test == "" {
			p.result = "ok  \t" + ev.Package
		}
	case "skip":
		if ev.Test == "" {
			p.result = "?   \t" + ev.Package + "\t[no test files]"
		}
	}
}

// write renders the output of the failed tests and the package result as go test prints them.
func (p *pkgEvents) write(out *strings.Builder) {
	// Subtests fail before their parents; report parents first.
	for i := len(p.failed) - 1; i >= 0; i-- {
		for _, l := range p.out[p.failed[i]] {
			out.WriteString(l + "\n")
		}
	}
	if p.result != "" {
		out.WriteString(p.result + "\n")
	}
}

func jsonToText(text string) (string, error) {
	pkgs := map[string]*pkgEvents{}
	var order []string
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue // build output interleaved with events
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return "", err
		}
		p, ok := pkgs[ev.Package]
		if !ok {
			p = &pkgEvents{out: map[string][]string{}}
			pkgs[ev.Package] = p
			order = append(order, ev.Package)
		}
		p.add(ev)
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, name := range order {
		pkgs[name].write(&b)
	}
	return b.String(), nil
}
