package cloudoptimizer

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/lukaszb/gascoigne/lazyre"
)

var (
	s3Object = lazyre.New(`^(\d{4}-\d\d-\d\d \d\d:\d\d:\d\d)\s+(\d+)\s+(.+)$`)
	s3Prefix = lazyre.New(`^\s*PRE (.+)$`)
)

type object struct {
	when string
	size int64
	key  string
}

// s3 summarizes object listings: small ones as "key size date", large ones
// as totals per top-level prefix and extension plus the newest and largest.
func (o *Optimizer) s3(lines []string) []string {
	var objs []object
	var out []string
	for _, l := range lines {
		if m := s3Object.FindStringSubmatch(l); m != nil {
			n, _ := strconv.ParseInt(m[2], 10, 64)
			objs = append(objs, object{m[1], n, m[3]})
		} else if m := s3Prefix.FindStringSubmatch(l); m != nil {
			out = append(out, "dir "+m[1])
		} else if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	if len(objs) <= o.cfg.MaxObjects {
		for _, ob := range objs {
			out = append(out, fmt.Sprintf("%s %s %s", ob.key, human(ob.size), ob.when))
		}
		return out
	}
	var total int64
	groups := map[string]int{}
	sizes := map[string]int64{}
	for _, ob := range objs {
		total += ob.size
		g := group(ob.key)
		groups[g]++
		sizes[g] += ob.size
	}
	out = append(out, fmt.Sprintf("%d objects, %s total", len(objs), human(total)))
	keys := slices.SortedFunc(maps.Keys(groups), func(a, b string) int {
		return cmp.Or(cmp.Compare(sizes[b], sizes[a]), cmp.Compare(a, b))
	})
	for _, k := range keys[:min(15, len(keys))] {
		out = append(out, fmt.Sprintf("  %s ×%d %s", k, groups[k], human(sizes[k])))
	}
	if len(keys) > 15 {
		out = append(out, fmt.Sprintf("  … %d more groups", len(keys)-15))
	}
	newest := slices.Clone(objs)
	slices.SortStableFunc(newest, func(a, b object) int { return cmp.Compare(b.when, a.when) })
	largest := slices.Clone(objs)
	slices.SortStableFunc(largest, func(a, b object) int { return cmp.Compare(b.size, a.size) })
	out = append(out, "newest:")
	for _, ob := range newest[:5] {
		out = append(out, fmt.Sprintf("  %s %s %s", ob.when, ob.key, human(ob.size)))
	}
	out = append(out, "largest:")
	for _, ob := range largest[:5] {
		out = append(out, fmt.Sprintf("  %s %s", human(ob.size), ob.key))
	}
	return out
}

// group is the top-level prefix and extension: "logs/*.json.gz".
func group(key string) string {
	top, _, nested := strings.Cut(key, "/")
	ext := path.Ext(key)
	if strings.HasSuffix(key, ".gz") {
		ext = path.Ext(strings.TrimSuffix(key, ".gz")) + ".gz"
	}
	if !nested {
		return "*" + ext
	}
	return top + "/*" + ext
}

func human(n int64) string {
	f := float64(n)
	for _, u := range []string{"B", "KB", "MB", "GB", "TB"} {
		if f < 1024 || u == "TB" {
			if u == "B" {
				return fmt.Sprintf("%d B", n)
			}
			return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") + " " + u
		}
		f /= 1024
	}
	return ""
}
