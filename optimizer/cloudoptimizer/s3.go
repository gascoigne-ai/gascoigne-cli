package cloudoptimizer

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/gascoigne-ai/gascoigne-cli/internal/compress"
	"github.com/gascoigne-ai/gascoigne-cli/internal/lazyre"
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
	objs, out := parseS3(lines)
	if len(objs) <= o.cfg.MaxObjects {
		for _, ob := range objs {
			out = append(out, fmt.Sprintf("%s %s %s", ob.key, compress.HumanBytes(ob.size, compress.LongBytes), ob.when))
		}
		return out
	}
	out = append(out, s3Groups(objs)...)
	return append(out, s3Extremes(objs)...)
}

// parseS3 reads the objects of a listing; the prefixes and other non-blank
// lines come back as output lines.
func parseS3(lines []string) (objs []object, out []string) {
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
	return objs, out
}

// s3Groups is the total and the biggest groups of a large listing.
func s3Groups(objs []object) []string {
	var total int64
	groups := map[string]int{}
	sizes := map[string]int64{}
	for _, ob := range objs {
		total += ob.size
		g := group(ob.key)
		groups[g]++
		sizes[g] += ob.size
	}
	out := []string{fmt.Sprintf("%d objects, %s total", len(objs), compress.HumanBytes(total, compress.LongBytes))}
	keys := slices.SortedFunc(maps.Keys(groups), func(left, right string) int {
		return cmp.Or(cmp.Compare(sizes[right], sizes[left]), cmp.Compare(left, right))
	})
	for _, k := range keys[:min(15, len(keys))] {
		out = append(out, fmt.Sprintf("  %s ×%d %s", k, groups[k], compress.HumanBytes(sizes[k], compress.LongBytes)))
	}
	if len(keys) > 15 {
		out = append(out, fmt.Sprintf("  … %d more groups", len(keys)-15))
	}
	return out
}

// s3Extremes lists the five newest and the five largest objects.
func s3Extremes(objs []object) []string {
	newest := slices.Clone(objs)
	slices.SortStableFunc(newest, func(left, right object) int { return cmp.Compare(right.when, left.when) })
	largest := slices.Clone(objs)
	slices.SortStableFunc(largest, func(left, right object) int { return cmp.Compare(right.size, left.size) })
	out := []string{"newest:"}
	for _, ob := range newest[:5] {
		out = append(out, fmt.Sprintf("  %s %s %s", ob.when, ob.key, compress.HumanBytes(ob.size, compress.LongBytes)))
	}
	out = append(out, "largest:")
	for _, ob := range largest[:5] {
		out = append(out, fmt.Sprintf("  %s %s", compress.HumanBytes(ob.size, compress.LongBytes), ob.key))
	}
	return out
}

// group is the top-level prefix and extension: "logs/*.json.gz".
func group(key string) string {
	top, _, nested := strings.Cut(key, "/")
	ext := path.Ext(key)
	if before, ok := strings.CutSuffix(key, ".gz"); ok {
		ext = path.Ext(before) + ".gz"
	}
	if !nested {
		return "*" + ext
	}
	return top + "/*" + ext
}
