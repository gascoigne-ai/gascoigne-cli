// Package version names this build of gascoigne: the release number and the
// git commit it was built from.
package version

import (
	"runtime/debug"
	"sync"
)

// Number is the release.
const Number = "0.2.0"

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

// String is "0.2.0 (abc1234)".
func String() string { return Number + " (" + Commit() + ")" }
