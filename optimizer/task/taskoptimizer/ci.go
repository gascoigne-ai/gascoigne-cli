package taskoptimizer

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// gh run view --log: "job<TAB>step<TAB>2024-10-10T10:00:00.1234567Z text"
	ciLine  = regexp.MustCompile(`^([^\t]+)\t([^\t]+)\t(?:\d{4}-\d\d-\d\dT[\d:.]+Z )?(.*)$`)
	ciError = regexp.MustCompile(`##\[error\]|Process completed with exit code [1-9]|^Error: `)
)

// ciLog: steps of CI jobs; passing steps become one line with their size,
// failing steps keep their output compressed by family.
func ciLog(lines []string) []string {
	var sections []section
	index := map[string]int{}
	var other []string
	for _, l := range lines {
		m := ciLine.FindStringSubmatch(l)
		if m == nil {
			other = append(other, l)
			continue
		}
		key := m[1] + " / " + m[2]
		i, ok := index[key]
		if !ok {
			i = len(sections)
			index[key] = i
			sections = append(sections, section{name: key, cmd: strings.TrimPrefix(m[2], "Run ")})
		}
		text := strings.TrimPrefix(m[3], "##[group]")
		if strings.HasPrefix(text, "##[endgroup]") {
			continue
		}
		if ciError.MatchString(text) {
			sections[i].failed = true
		}
		sections[i].body = append(sections[i].body, text)
	}
	for i, s := range sections {
		if !s.failed {
			sections[i].name = fmt.Sprintf("%s (%d lines)", s.name, len(s.body))
		}
	}
	return render(other, sections, nil)
}
