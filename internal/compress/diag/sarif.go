package diag

import (
	"encoding/json"
	"fmt"
	"strings"
)

// sarifLog is the part of a SARIF log SARIF reads.
type sarifLog struct {
	Runs []struct {
		Tool struct {
			Driver struct {
				Name string `json:"name"`
			} `json:"driver"`
		} `json:"tool"`
		Results []sarifResult `json:"results"`
	} `json:"runs"`
}

type sarifResult struct {
	RuleID  string `json:"ruleId"`
	Level   string `json:"level"`
	Message struct {
		Text string `json:"text"`
	} `json:"message"`
	Locations []struct {
		Physical struct {
			Artifact struct {
				URI string `json:"uri"`
			} `json:"artifactLocation"`
			Region struct {
				Line int `json:"startLine"`
				Col  int `json:"startColumn"`
			} `json:"region"`
		} `json:"physicalLocation"`
	} `json:"locations"`
}

// SARIF turns a SARIF log (CodeQL, semgrep, eslint, gosec and other
// analyzers) into compiler-style lines, file:line:col: level: message
// [rule], so Compress groups them by file. Rule metadata, tool details and
// fingerprints go; each result keeps its location, level, rule and message.
func SARIF(text string) ([]string, error) {
	var log sarifLog
	if err := json.Unmarshal([]byte(text), &log); err != nil {
		return nil, fmt.Errorf("sarif: %w", err)
	}
	var out []string
	for _, run := range log.Runs {
		out = append(out, fmt.Sprintf("%s: %d results", run.Tool.Driver.Name, len(run.Results)))
		for _, r := range run.Results {
			out = append(out, r.line())
		}
	}
	return out, nil
}

// line renders the result as file:line:col: level: message [rule], without the position when it has none.
func (r sarifResult) line() string {
	level := r.Level
	if level == "" {
		level = "warning" // the SARIF default
	}
	if level == "none" {
		level = "note"
	}
	msg, _, _ := strings.Cut(strings.TrimSpace(r.Message.Text), "\n")
	if r.RuleID != "" {
		msg += " [" + r.RuleID + "]"
	}
	if len(r.Locations) == 0 {
		return level + ": " + msg
	}
	loc := r.Locations[0].Physical
	uri := strings.TrimPrefix(loc.Artifact.URI, "file://")
	return fmt.Sprintf("%s:%d:%d: %s: %s", uri, max(loc.Region.Line, 1), max(loc.Region.Col, 1), level, msg)
}
