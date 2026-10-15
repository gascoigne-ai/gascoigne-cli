package testreport

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
)

// fileCover is one file's coverage.
type fileCover struct {
	name       string
	pct        float64
	hit, total int      // lines (lcov); 0 when only a percentage is known
	uncovered  []int    // uncovered line numbers (lcov)
	weak       []string // functions under the threshold with their percent (go)
}

// goFunc is a go tool cover -func line: path/file.go:12:	Name	85.7%
var goFunc = lazyre.New(`^(\S+?\.go):\d+:\s+\S+\s+([\d.]+)%$`)

// Coverage renders a coverage report (lcov tracefile, go tool cover -func)
// as the total plus the files under below percent, worst first, with their
// uncovered line ranges when the report lists lines; the rest are counted.
func Coverage(lines []string, below float64) []string {
	files, total := lcov(lines)
	if len(files) == 0 {
		files, total = goCover(lines, below)
	}
	if len(files) == 0 {
		return lines
	}
	slices.SortStableFunc(files, func(a, b fileCover) int { return cmp.Or(cmp.Compare(a.pct, b.pct), strings.Compare(a.name, b.name)) })
	out := []string{fmt.Sprintf("total: %s of %d files", pct(total), len(files))}
	low := 0
	for _, f := range files {
		if f.pct >= below {
			continue
		}
		low++
		l := f.name + " " + pct(f.pct)
		if f.total > 0 {
			l += fmt.Sprintf(" (%d/%d lines)", f.hit, f.total)
		}
		if r := ranges(f.uncovered); r != "" {
			l += " uncovered: " + r
		}
		if len(f.weak) > 0 {
			l += ": " + strings.Join(f.weak, ", ")
		}
		out = append(out, l)
	}
	if n := len(files) - low; n > 0 {
		out = append(out, fmt.Sprintf("%d files at or above %s", n, pct(below)))
	}
	return out
}

// lcov reads SF/DA/LF/LH records.
func lcov(lines []string) ([]fileCover, float64) {
	var files []fileCover
	var cur *fileCover
	hit, total := 0, 0
	for _, l := range lines {
		key, val, _ := strings.Cut(strings.TrimSpace(l), ":")
		switch key {
		case "SF":
			files = append(files, fileCover{name: val})
			cur = &files[len(files)-1]
		case "DA":
			if cur == nil {
				continue
			}
			num, count, _ := strings.Cut(val, ",")
			n, _ := strconv.Atoi(num)
			if c, _ := strconv.Atoi(strings.Split(count, ",")[0]); c == 0 {
				cur.uncovered = append(cur.uncovered, n)
			}
		case "LF":
			if cur != nil {
				cur.total, _ = strconv.Atoi(val)
			}
		case "LH":
			if cur != nil {
				cur.hit, _ = strconv.Atoi(val)
			}
		case "end_of_record":
			if cur != nil && cur.total > 0 {
				cur.pct = 100 * float64(cur.hit) / float64(cur.total)
				hit, total = hit+cur.hit, total+cur.total
			} else if cur != nil {
				cur.pct = 100
			}
			cur = nil
		}
	}
	if total == 0 {
		return files, 100
	}
	return files, 100 * float64(hit) / float64(total)
}

// goCover averages function percentages per file, naming the functions
// under below; the report's own total line is kept as the total.
func goCover(lines []string, below float64) ([]fileCover, float64) {
	sums := map[string][]float64{}
	weak := map[string][]string{}
	var order []string
	total := -1.0
	for _, l := range lines {
		if f := strings.Fields(l); len(f) == 3 && f[0] == "total:" {
			total, _ = strconv.ParseFloat(strings.TrimSuffix(f[2], "%"), 64)
			continue
		}
		m := goFunc.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		v, _ := strconv.ParseFloat(m[2], 64)
		if _, ok := sums[m[1]]; !ok {
			order = append(order, m[1])
		}
		sums[m[1]] = append(sums[m[1]], v)
		if v < below {
			weak[m[1]] = append(weak[m[1]], strings.Fields(l)[1]+" "+m[2]+"%")
		}
	}
	var files []fileCover
	for _, name := range order {
		vs := sums[name]
		sum := 0.0
		for _, v := range vs {
			sum += v
		}
		files = append(files, fileCover{name: name, pct: sum / float64(len(vs)), weak: weak[name]})
	}
	return files, total
}

// ranges joins sorted line numbers into runs: 3-7,12,40-41.
func ranges(nums []int) string {
	var parts []string
	for i := 0; i < len(nums); {
		j := i
		for j+1 < len(nums) && nums[j+1] == nums[j]+1 {
			j++
		}
		if j > i {
			parts = append(parts, fmt.Sprintf("%d-%d", nums[i], nums[j]))
		} else {
			parts = append(parts, strconv.Itoa(nums[i]))
		}
		i = j + 1
	}
	if len(parts) > 12 {
		return strings.Join(parts[:12], ",") + " " + compress.Omitted(len(parts)-12, "ranges")
	}
	return strings.Join(parts, ",")
}

func pct(v float64) string { return strconv.FormatFloat(v, 'f', 1, 64) + "%" }
