package testreport

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
)

// junitCase is one <testcase>; Failure, Error and Skipped hold its result.
type junitCase struct {
	Class   string        `xml:"classname,attr"`
	Name    string        `xml:"name,attr"`
	File    string        `xml:"file,attr"`
	Failure *junitProblem `xml:"failure"`
	Error   *junitProblem `xml:"error"`
	Skipped *junitProblem `xml:"skipped"`
	Out     string        `xml:"system-out"`
	Err     string        `xml:"system-err"`
}

type junitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Text    string `xml:",chardata"`
}

type junitSuite struct {
	Name   string       `xml:"name,attr"`
	Time   string       `xml:"time,attr"`
	Cases  []junitCase  `xml:"testcase"`
	Suites []junitSuite `xml:"testsuite"`
}

// JUnit renders a JUnit XML report (pytest, maven surefire, jest-junit,
// gotestsum and most CI tools write it): per suite a count line, then every
// failing or erroring case with its message, details and output; skipped
// cases are named with their reason. Passing cases become counts.
func JUnit(text string) ([]string, error) {
	var root struct {
		XMLName xml.Name
		junitSuite
	}
	if err := xml.Unmarshal([]byte(text), &root); err != nil {
		return nil, fmt.Errorf("junit: %w", err)
	}
	suites := root.Suites
	if root.XMLName.Local == "testsuite" {
		suites = []junitSuite{root.junitSuite}
	}
	var out []string
	for _, s := range suites {
		out = append(out, suite(s)...)
	}
	return out, nil
}

func suite(s junitSuite) []string {
	var out, details []string
	passed, failed, skipped := 0, 0, 0
	for _, c := range s.Cases {
		name := strings.TrimPrefix(c.Class+"::"+c.Name, "::")
		switch {
		case c.Failure != nil || c.Error != nil:
			failed++
			p, kind := c.Failure, "FAIL"
			if p == nil {
				p, kind = c.Error, "ERROR"
			}
			details = append(details, problem(kind, name, p)...)
			for _, o := range []string{c.Out, c.Err} {
				if o = strings.TrimSpace(o); o != "" {
					details = append(details, indent(compress.Lines(o))...)
				}
			}
		case c.Skipped != nil:
			skipped++
			line := strings.TrimSpace("SKIP " + name + " " + c.Skipped.Message)
			if text := strings.TrimSpace(c.Skipped.Text); text != "" && text != strings.TrimSpace(c.Skipped.Message) {
				line += " (" + firstLine(text) + ")"
			}
			details = append(details, line)
		default:
			passed++
		}
	}
	head := fmt.Sprintf("%s: %d passed, %d failed, %d skipped", s.Name, passed, failed, skipped)
	if s.Time != "" {
		head += " (" + s.Time + "s)"
	}
	if len(s.Cases) > 0 {
		out = append(out, head)
	}
	out = append(out, details...)
	for _, sub := range s.Suites {
		out = append(out, suite(sub)...)
	}
	return out
}

func problem(kind, name string, p *junitProblem) []string {
	head := kind + " " + name
	if msg := strings.TrimSpace(p.Message); msg != "" {
		head += ": " + firstLine(msg)
	}
	out := []string{head}
	text := strings.TrimSpace(p.Text)
	if text != "" && text != strings.TrimSpace(p.Message) {
		out = append(out, indent(compress.Lines(text))...)
	}
	return out
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

func indent(lines []string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = "  " + l
	}
	return out
}
