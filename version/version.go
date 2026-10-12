// Package version names this build of gascoigne: the release number and the
// git commit it was built from.
package version

import (
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
)

// Number is the release.
const Number = "0.3.0"

// Commit is the short git hash the binary was built from, with "-dirty"
// when the work tree had uncommitted changes, or "unknown" when the build
// recorded none (go run, tests, builds outside a checkout).
var Commit = sync.OnceValue(func() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	return commit(info.Settings)
})

func commit(settings []debug.BuildSetting) string {
	rev, dirty := "", false
	for _, s := range settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value[:min(7, len(s.Value))]
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	switch {
	case rev == "":
		return "unknown"
	case dirty:
		return rev + "-dirty"
	}
	return rev
}

// String is "0.2.1 (abc1234)".
func String() string { return Number + " (" + Commit() + ")" }

// Newer reports whether release a is a later version than b. Both are
// MAJOR.MINOR.PATCH with an optional -suffix: a suffixed version (0.2.2-dev)
// comes before its plain release (0.2.2). Text that is not a version is
// never newer, and nothing is newer than it.
func Newer(a, b string) bool {
	x, okx := parse(a)
	y, oky := parse(b)
	if !okx || !oky {
		return false
	}
	for i := range 3 {
		if x.n[i] != y.n[i] {
			return x.n[i] > y.n[i]
		}
	}
	return !x.pre && y.pre
}

type parsed struct {
	n   [3]int
	pre bool // has a -suffix: before the release itself
}

func parse(s string) (p parsed, ok bool) {
	core, suffix, _ := strings.Cut(s, "-")
	p.pre = suffix != ""
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return p, false
	}
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return p, false
		}
		p.n[i] = n
	}
	return p, true
}
