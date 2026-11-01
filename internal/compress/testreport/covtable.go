package testreport

import (
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

var (
	// pyHeader: coverage.py and pytest-cov, "Name Stmts Miss [Branch BrPart] Cover [Missing]".
	pyHeader = lazyre.New(`^Name\s+Stmts\s+Miss\s+(Branch\s+BrPart\s+)?Cover\b`)
	// istHeader: istanbul text reporter of jest, nyc, c8 and vitest.
	istHeader = lazyre.New(`^File\s*\|\s*% Stmts\s*\|`)
	// tableRule: the separator lines of both tables.
	tableRule = lazyre.New(`^-{5,}(\|-+)*\s*$`)
)

// covTable is a coverage table found among other lines.
type covTable struct {
	files      []fileCover
	total      float64
	start, end int // the table occupies lines[start:end]
}

// CoverageTables replaces each coverage table (coverage.py, pytest-cov,
// jest, nyc, c8, vitest) in lines with the files under the threshold and
// the total; the other lines stay, so a test run keeps its failures.
func CoverageTables(lines []string, below float64) []string {
	var out []string
	for {
		t, ok := findTable(lines)
		if !ok {
			return append(out, lines...)
		}
		out = append(out, lines[:t.start]...)
		out = append(out, render(t.files, t.total, below)...)
		lines = lines[t.end:]
	}
}

func findTable(lines []string) (covTable, bool) {
	for i, l := range lines {
		var t covTable
		switch {
		case pyHeader.MatchString(l):
			t = pyTable(lines, i)
		case istHeader.MatchString(l):
			t = istTable(lines, i)
		}
		if len(t.files) > 0 {
			return t, true
		}
	}
	return covTable{}, false
}

// pyTable reads the rows after the header at i up to the TOTAL row.
func pyTable(lines []string, i int) covTable {
	t := covTable{start: i, total: -1}
	j := i + 1
	for ; j < len(lines); j++ {
		l := strings.TrimSpace(lines[j])
		if tableRule.MatchString(l) {
			continue
		}
		f := strings.Fields(l)
		k := coverCol(f)
		if len(f) < 4 || k < 0 {
			break
		}
		v, _ := strconv.ParseFloat(strings.TrimSuffix(f[k], "%"), 64)
		if f[0] == "TOTAL" {
			t.total, j = v, j+1
			break
		}
		t.files = append(t.files, fileCover{name: f[0], pct: v,
			missing: capRanges(strings.FieldsFunc(strings.Join(f[k+1:], " "), isListSep))})
	}
	t.end = j
	t.total = totalOr(t.total, t.files)
	return t
}

// coverCol is the index of the first percent after the name.
func coverCol(f []string) int {
	for k := 1; k < len(f); k++ {
		if strings.HasSuffix(f[k], "%") {
			return k
		}
	}
	return -1
}

// istTable reads the rows after the header at i. A directory row (indent 1,
// followed by a file at indent 2) gives the path prefix of the files below it.
func istTable(lines []string, i int) covTable {
	t := covTable{start: i, total: -1}
	if i > 0 && tableRule.MatchString(lines[i-1]) {
		t.start = i - 1
	}
	var rows [][]string
	j := i + 1
	for ; j < len(lines) && strings.Contains(lines[j], "|"); j++ {
		if !tableRule.MatchString(lines[j]) {
			rows = append(rows, strings.Split(lines[j], "|"))
		}
	}
	if j < len(lines) && tableRule.MatchString(lines[j]) {
		j++
	}
	t.end = j
	dir := ""
	for k, c := range rows {
		if len(c) < 5 {
			continue
		}
		name := strings.TrimSpace(c[0])
		v, err := strconv.ParseFloat(strings.TrimSpace(c[4]), 64) // % Lines
		switch {
		case err != nil:
		case name == "All files":
			t.total = v
		case leadingSpaces(c[0]) == 1 && k+1 < len(rows) && leadingSpaces(rows[k+1][0]) == 2:
			dir = name
		default:
			if leadingSpaces(c[0]) < 2 {
				dir = ""
			}
			f := fileCover{name: name, pct: v}
			if dir != "" {
				f.name = dir + "/" + name
			}
			if len(c) > 5 {
				f.missing = capRanges(strings.FieldsFunc(c[5], isListSep))
			}
			t.files = append(t.files, f)
		}
	}
	t.total = totalOr(t.total, t.files)
	return t
}

func leadingSpaces(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

func isListSep(r rune) bool { return r == ',' || r == ' ' }

// totalOr is the mean of the files when the table has no total row.
func totalOr(total float64, files []fileCover) float64 {
	if total >= 0 || len(files) == 0 {
		return total
	}
	sum := 0.0
	for _, f := range files {
		sum += f.pct
	}
	return sum / float64(len(files))
}
