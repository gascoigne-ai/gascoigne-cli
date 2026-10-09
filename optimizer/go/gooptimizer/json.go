package gooptimizer

import (
	"bufio"
	"encoding/json"
	"strings"
)

type event struct {
	Action  string
	Package string
	Test    string
	Output  string
	Elapsed float64
}

// jsonToText turns go test -json events back into plain go test output,
// keeping only output of failed tests and package results.
func jsonToText(text string) (string, error) {
	type pkg struct {
		out    map[string][]string // test → output
		failed []string
		result string
	}
	pkgs := map[string]*pkg{}
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
			p = &pkg{out: map[string][]string{}}
			pkgs[ev.Package] = p
			order = append(order, ev.Package)
		}
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
	if err := sc.Err(); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, name := range order {
		p := pkgs[name]
		// Subtests fail before their parents; report parents first.
		for i := len(p.failed) - 1; i >= 0; i-- {
			for _, l := range p.out[p.failed[i]] {
				b.WriteString(l + "\n")
			}
		}
		if p.result != "" {
			b.WriteString(p.result + "\n")
		}
	}
	return b.String(), nil
}
